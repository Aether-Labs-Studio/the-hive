package main

import (
	"context"
	"crypto/ed25519"
	"flag"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"the-hive/internal/dht"
	"the-hive/internal/logger"
	"the-hive/internal/mcp"
	"the-hive/internal/sanitizer"
	"the-hive/internal/utils"
	"time"
)

var version = "dev"

func main() {
	os.Exit(runServe(os.Args[1:]))
}

func runServe(args []string) int {
	fs := flag.NewFlagSet("hive", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	addr := fs.String("addr", "127.0.0.1:0", "UDP address to listen on for DHT")
	nodeIDStr := fs.String("node-id", "", "Custom ID for the local node (Optional, overrides identity)")
	bootstrapAddr := fs.String("bootstrap", "", "Address of the bootstrap node (IP:Port)")
	monitorPort := fs.Int("monitor-port", defaultMonitorPort, "Local port for the HTTP/SSE telemetry monitor")
	autoDiscovery := fs.Bool("auto-discovery", defaultAutoDiscovery, "Enable automatic peer discovery on local network (multicast)")
	discoveryPort := fs.Int("discovery-port", defaultDiscoveryPort, "UDP port for multicast discovery")
	maxStorage := fs.Int64("max-storage", defaultMaxStorage, "Maximum disk storage in bytes (default 1GB)")
	if err := fs.Parse(args); err != nil {
		logger.Error("Error al parsear flags: %v", err)
		return 1
	}

	explicitFlags := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		explicitFlags[f.Name] = true
	})

	dataDir, err := ensureDataDir()
	if err != nil {
		logger.Error("Error al preparar el directorio de datos: %v", err)
		return 1
	}

	configPath := filepath.Join(dataDir, "config.json")
	fileCfg, err := ensureConfigFile(configPath)
	if err != nil {
		logger.Error("Error al preparar config.json: %v", err)
		return 1
	}

	effectiveCfg := resolveRuntimeConfig(fileCfg, cliConfig{
		Addr:          *addr,
		Bootstrap:     *bootstrapAddr,
		MonitorPort:   *monitorPort,
		AutoDiscovery: *autoDiscovery,
		DiscoveryPort: *discoveryPort,
		MaxStorage:    *maxStorage,
	}, explicitFlags)

	privateKey, localID, err := loadLocalIdentity(dataDir, *nodeIDStr)
	if err != nil {
		logger.Error("Error al cargar identidad: %v", err)
		return 1
	}

	rulesPath := filepath.Join(dataDir, "rules.json")
	if err := ensureRulesFile(rulesPath); err != nil {
		logger.Error("Error al preparar rules.json: %v", err)
		return 1
	}

	repPath := filepath.Join(dataDir, "reputation.json")
	repStore, err := dht.NewReputationStore(repPath)
	if err != nil {
		logger.Error("Error al inicializar el almacén de reputación: %v", err)
		return 1
	}

	logger.Info("Iniciando The Hive: Infraestructura de Conciencia Compartida")
	logger.Info("DEBUG Identidad: %x", localID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	storage, err := dht.NewDiskStorage(dataDir, effectiveCfg.MaxStorage, repStore)
	if err != nil {
		logger.Error("Error al inicializar el almacenamiento en disco: %v", err)
		return 1
	}
	logger.Info("Almacenamiento persistente activado en: %s (Límite local: %d bytes)", dataDir, effectiveCfg.MaxStorage)

	rt := dht.NewRoutingTable(localID)
	transport := dht.NewTransport(localID, nil)

	router := dht.NewRouter(transport, rt, storage)
	transport.SetHandler(router)
	engine := dht.NewEngine(router, repStore)
	router.SetSubscriptionManager(engine.GetSubscriptionManager())
	engine.SetSwarmContext(ctx)
	engine.StartWorkers(ctx, dht.DefaultWorkerOptions)

	dht.GlobalTelemetry.SetEngine(engine)
	go dht.StartMonitor("127.0.0.1:" + strconv.Itoa(effectiveCfg.MonitorPort))

	sentinel, err := sanitizer.NewSentinel(rulesPath, privateKey)
	if err != nil {
		logger.Error("Error al inicializar Sentinel: %v", err)
		return 1
	}
	engine.SetSanitizer(sentinel)

	router.SetSigner(sentinel)
	mcpServer := mcp.NewServer(engine, sentinel)

	if err := transport.Listen(effectiveCfg.Addr); err != nil {
		logger.Error("Error al iniciar el transporte UDP: %v", err)
		return 1
	}
	logger.Info("DHT escuchando en: %s (ID: %x)", transport.Addr().String(), localID)

	dhtPort := transport.Addr().(*net.UDPAddr).Port
	mcastAddr := "239.0.0.1:" + strconv.Itoa(effectiveCfg.DiscoveryPort)
	discovery := dht.NewDiscovery(router, dhtPort, mcastAddr, effectiveCfg.AutoDiscovery)
	discovery.Start(ctx)
	if effectiveCfg.Bootstrap != "" {
		logger.Info("DEBUG: Launching bootstrap goroutine...")
		go func() {
			time.Sleep(100 * time.Millisecond)
			logger.Info("DEBUG: Goroutine waking up, calling engine.Bootstrap...")
			if err := engine.Bootstrap(ctx, effectiveCfg.Bootstrap); err != nil {
				logger.Error("Error en el proceso de Bootstrap: %v", err)
			}
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		logger.Info("Servidor MCP iniciado (esperando peticiones por Stdin)")
		mcpServer.Serve()
		sigChan <- syscall.SIGTERM
	}()

	sig := <-sigChan
	logger.Info("Señal recibida (%v). Iniciando apagado seguro...", sig)
	cancel()

	if err := transport.Stop(); err != nil {
		logger.Error("Error al detener el transporte: %v", err)
	} else {
		logger.Info("Transporte UDP detenido limpiamente.")
	}

	logger.Info("The Hive: Base de infraestructura finalizada.")
	return 0
}

func ensureDataDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dataDir := filepath.Join(homeDir, ".hive_data")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return "", err
	}
	return dataDir, nil
}

func loadLocalIdentity(dataDir, nodeIDOverride string) (ed25519.PrivateKey, dht.NodeID, error) {
	identityPath := filepath.Join(dataDir, "identity.pem")
	privateKey, err := utils.LoadOrGenerateIdentity(identityPath)
	if err != nil {
		return nil, dht.NodeID{}, err
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	localID := utils.DeriveNodeID(publicKey)
	if nodeIDOverride != "" {
		localID = dht.NewNodeID(nodeIDOverride)
	}
	return privateKey, localID, nil
}

func ensureRulesFile(rulesPath string) error {
	if _, err := os.Stat(rulesPath); os.IsNotExist(err) {
		defaultRules := `{
  "redact_patterns": [
    "(?i)api_key",
    "(?i)password",
    "(?i)secret"
  ]
}
`
		if err := os.WriteFile(rulesPath, []byte(defaultRules), 0600); err != nil {
			return err
		}
		logger.Info("Archivo rules.json creado automáticamente en: %s", rulesPath)
	}
	return nil
}
