package dht

import (
	"context"
	"crypto/rand"
	"fmt"
	"the-hive/internal/logger"
	"time"
)

// WorkerOptions contains the configuration for the maintenance workers.
type WorkerOptions struct {
	RefreshInterval     time.Duration
	ReplicationInterval time.Duration
	TopologyInterval    time.Duration
	GCInterval          time.Duration
}

// DefaultWorkerOptions provides a standard configuration for workers.
var DefaultWorkerOptions = WorkerOptions{
	RefreshInterval:     30 * time.Minute,
	ReplicationInterval: 1 * time.Hour,
	TopologyInterval:    5 * time.Second,
	GCInterval:          10 * time.Minute,
}

// StartWorkers initiates the background maintenance processes for the DHT.
func (e *Engine) StartWorkers(ctx context.Context, opts WorkerOptions) {
	logger.Info("Workers: Iniciando trabajadores de mantenimiento (Refresco: %v, Replicación: %v, Topología: %v, GC: %v)", 
		opts.RefreshInterval, opts.ReplicationInterval, opts.TopologyInterval, opts.GCInterval)

	// Worker de Refresco de Buckets
	go e.refreshWorker(ctx, opts.RefreshInterval)

	// Worker de Replicación de Datos (Periódica)
	go e.replicationWorker(ctx, opts.ReplicationInterval)

	// Worker de Redistribución (Proactiva/Handoff)
	go e.handoffWorker(ctx)

	// Worker de Topología (Live Monitor)
	go e.topologyWorker(ctx, opts.TopologyInterval)

	// Worker de Limpieza (GC)
	go e.gcWorker(ctx, opts.GCInterval)
}

func (e *Engine) gcWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Workers: Deteniendo trabajador de limpieza (GC).")
			return
		case <-ticker.C:
			deleted := e.router.storage.CleanExpired()
			if deleted > 0 {
				logger.Info("Workers: Ciclo de limpieza completado. Chunks eliminados: %d", deleted)
				GlobalTelemetry.Emit(EventType("StorageCleaned"), "", fmt.Sprintf("Deleted %d expired chunks", deleted))
			}
		}
	}
}

func (e *Engine) handoffWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.Info("Workers: Deteniendo trabajador de redistribución (handoff).")
			return
		case contact := <-e.router.newContactChan:
			// Redispense relevant chunks to the new contact
			go e.proactiveReplication(contact)
		}
	}
}

// proactiveReplication redistributes local chunks to a new contact if it's
// among the k-closest nodes for those chunks.
func (e *Engine) proactiveReplication(contact Contact) {
	keys := e.router.storage.GetAllKeys()
	if len(keys) == 0 {
		return
	}

	for _, key := range keys {
		// Phase 1.1.0: Only replicate Committed chunks
		if meta, ok := e.router.storage.GetMetadata(key); ok {
			if meta.State != StateCommitted { continue }
		}

		// Calculate if the new node belongs in the k-closest set for this key.
		closest := e.router.rt.FindClosestContacts(key, K)
		
		isClose := false
		for _, c := range closest {
			if c.ID == contact.ID {
				isClose = true
				break
			}
		}

		if isClose {
			data, ok := e.router.storage.Retrieve(key)
			if !ok {
				continue
			}
			// Send STORE to the new contact
			if err := e.router.StoreValueRemote(contact.Address, key, data); err == nil {
				GlobalTelemetry.Emit(DataReplicated, fmt.Sprintf("%x", contact.ID), fmt.Sprintf("Handoff key %x", key))
			}
		}
	}
}

func (e *Engine) topologyWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Workers: Deteniendo trabajador de topología.")
			return
		case <-ticker.C:
			e.emitTopology()
		}
	}
}

func (e *Engine) refreshWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Workers: Deteniendo trabajador de refresco.")
			return
		case <-ticker.C:
			logger.Debug("Workers: Iniciando refresco periódico de k-buckets.")
			e.refreshBuckets()
		}
	}
}

func (e *Engine) refreshBuckets() {
	// For each bucket index, generate a random ID and perform a lookup
	for i := 0; i < 160; i++ {
		targetID := e.generateRandomIDInBucket(i)
		_, _ = e.router.LookupNode(targetID)
	}
	GlobalTelemetry.Emit(BucketRefresh, "", "Periodic refresh of all 160 k-buckets completed")
}

func (e *Engine) generateRandomIDInBucket(index int) NodeID {
	id := e.router.sender.LocalID()
	byteIdx := 19 - (index / 8)
	bitIdx := index % 8
	
	id[byteIdx] ^= (1 << bitIdx)
	
	for i := 0; i < byteIdx; i++ {
		rand.Read(id[i:i+1])
	}
	mask := byte((1 << bitIdx) - 1)
	var randByte [1]byte
	rand.Read(randByte[:])
	id[byteIdx] = (id[byteIdx] & ^mask) | (randByte[0] & mask)
	
	return id
}

func (e *Engine) replicationWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Workers: Deteniendo trabajador de replicación.")
			return
		case <-ticker.C:
			logger.Info("Workers: Iniciando ciclo de replicación de datos.")
			e.replicateData()
		}
	}
}

func (e *Engine) replicateData() {
	keys := e.router.storage.GetAllKeys()
	if len(keys) == 0 {
		return
	}

	for _, key := range keys {
		// Phase 1.1.0: Only replicate Committed chunks
		if meta, ok := e.router.storage.GetMetadata(key); ok {
			if meta.State != StateCommitted { continue }
		}

		data, ok := e.router.storage.Retrieve(key)
		if !ok {
			continue
		}

		closest := e.router.rt.FindClosestContacts(key, K)
		for _, contact := range closest {
			if contact.ID == e.router.sender.LocalID() {
				continue
			}

			go func(c Contact, k NodeID, d []byte) {
				if err := e.router.StoreValueRemote(c.Address, k, d); err == nil {
					GlobalTelemetry.Emit(DataReplicated, fmt.Sprintf("%x", c.ID), fmt.Sprintf("Replicated key %x", k))
				}
			}(contact, key, data)
		}
	}
}
