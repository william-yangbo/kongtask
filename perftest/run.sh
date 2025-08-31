#!/usr/bin/env bash
set -e

echo "🚀 KongTask Performance Test Runner"
echo "=================================="
echo ""

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo "❌ Error: Docker is not running. Please start Docker and try again."
    exit 1
fi

echo "✅ Docker is running"
echo ""

# Change to kongtask root directory
cd "$(dirname "$0")/.."

echo "🧪 Running Performance Tests..."
echo "==============================="
echo ""

# Run startup/shutdown performance test
echo "1️⃣  Testing Worker Startup/Shutdown Performance..."
go test -v ./perftest -run TestStartupShutdownPerformance -timeout 5m
echo ""

# Run latency performance test
echo "2️⃣  Testing Job Latency Performance..."
go test -v ./perftest -run TestLatencyPerformance -timeout 5m
echo ""

# Run concurrency performance test
echo "3️⃣  Testing Concurrency Performance..."
go test -v ./perftest -run TestConcurrencyPerformance -timeout 10m
echo ""

# Run bulk jobs performance test (equivalent to original run.sh)
echo "4️⃣  Testing Bulk Jobs Performance (20,000 jobs)..."
go test -v ./perftest -run TestBulkJobsPerformance -timeout 15m
echo ""

# Run parallel worker performance test (equivalent to v0.4.0 run.js)
echo "5️⃣  Testing Parallel Worker Performance (4 workers, 20,000 jobs)..."
go test -v ./perftest -run TestParallelWorkerPerformance -timeout 15m
echo ""

echo "🎉 All Performance Tests Completed!"
echo ""
echo "📊 Summary:"
echo "  - Startup/Shutdown: Worker lifecycle performance"
echo "  - Latency Analysis: 1,000 jobs with detailed timing"
echo "  - Concurrency Scaling: 1,2,4,8 worker comparison"
echo "  - Bulk Processing: 20,000 jobs with 10 workers"
echo "  - Parallel Workers: 4 workers matching v0.4.0 run.js"
echo ""
echo "✨ KongTask Performance Testing Complete!"
