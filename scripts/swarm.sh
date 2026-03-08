#!/bin/bash

# The Hive - Swarm Test Script
# This script starts a local P2P network of 5 nodes.

# 1. Build the binary
echo "[HIVE] Building 'the-hive' binary..."
go build -o hive ./cmd/hive/main.go

# 2. Configuration
SEED_PORT=8000
NUM_NODES=5
LOG_DIR="./swarm_logs"

mkdir -p $LOG_DIR
rm -rf $LOG_DIR/*.log

# 3. Start Node 1 (Seed Node)
echo "[HIVE] Starting Node 1 (Seed) on port $SEED_PORT..."
tail -f /dev/null | ./hive -addr 127.0.0.1:$SEED_PORT -node-id node-seed serve 2> "$LOG_DIR/node1.log" &
PID_LIST=$!

# Wait for seed to be ready
sleep 1

# 4. Start Additional Nodes (Peers)
for i in $(seq 2 $NUM_NODES); do
    NODE_ID="node-$i"
    echo "[HIVE] Starting Node $i ($NODE_ID) bootstrapping from 127.0.0.1:$SEED_PORT..."
    # Using addr 127.0.0.1:0 for dynamic OS port
    tail -f /dev/null | ./hive -node-id $NODE_ID -bootstrap 127.0.0.1:$SEED_PORT serve 2> "$LOG_DIR/node$i.log" &
    PID_LIST="$PID_LIST $!"
done

echo "[HIVE] Swarm of $NUM_NODES nodes is running."
echo "[HIVE] Monitoring telemetry (JSONL) for 5 seconds..."
sleep 5
echo "[HIVE] Current Telemetry events in node1.log:"
grep "type" "$LOG_DIR/node1.log" || echo "(No events yet)"

# 5. Trap SIGINT to stop all nodes
trap "echo -e '\n[HIVE] Stopping swarm...'; kill $PID_LIST; exit" SIGINT

# Keep script alive but without blocking tail
echo "[HIVE] Swarm is running in background. Press Ctrl+C to stop."
while true; do sleep 1; done
