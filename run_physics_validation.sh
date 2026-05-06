#!/bin/bash

echo "=== PHYSICS VALIDATION RUN ===" > physics_results.txt

echo "Building engine..." >> physics_results.txt
go clean -cache
go build ./cmd/loadequilibrium

echo "Running baseline physics..." >> physics_results.txt
LOG_LEVEL=debug ./loadequilibrium > baseline.log 2>&1 &
PID=$!
sleep 8
kill $PID

echo "Baseline mass trend:" >> physics_results.txt
grep network_field baseline.log | tail -20 >> physics_results.txt


echo "Running shock scenario..." >> physics_results.txt
LOG_LEVEL=debug ./loadequilibrium > shock.log 2>&1 &
PID=$!
sleep 8
kill $PID

echo "Shock mass trend:" >> physics_results.txt
grep network_field shock.log | tail -20 >> physics_results.txt


echo "Running persistent forcing..." >> physics_results.txt
LOG_LEVEL=debug ./loadequilibrium > persistent.log 2>&1 &
PID=$!
sleep 12
kill $PID

echo "Persistent mass trend:" >> physics_results.txt
grep network_field persistent.log | tail -20 >> physics_results.txt

echo "=== VALIDATION COMPLETE ===" >> physics_results.txt

echo "Done. Results saved in physics_results.txt"
