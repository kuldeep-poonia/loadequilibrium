.PHONY: build run clean docker-up docker-down elite-test elite-test-validate

GO ?= go
BIN_DIR ?= bin
BINARY ?= loadequilibrium
PYTHON ?= python3

build:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o $(BIN_DIR)/$(BINARY) ./cmd/loadequilibrium/

run:
	$(GO) run ./cmd/loadequilibrium/

clean:
	rm -rf $(BIN_DIR)/

# Docker Management Targets
docker-up:
	@echo "[Docker] Starting LoadEquilibrium stack..."
	docker-compose up -d
	@echo "[Docker] Waiting for services to be ready..."
	@sleep 10
	@echo "[Docker] Stack is running. Services:"
	@echo "  • LoadEquilibrium: http://localhost:8080"
	@echo "  • Prometheus: http://localhost:9090"
	@echo "  • Metrics App: http://localhost:8000"
	@docker-compose ps

docker-down:
	@echo "[Docker] Stopping LoadEquilibrium stack..."
	docker-compose down
	@echo "[Docker] Stack stopped"

docker-logs:
	@docker-compose logs -f loadequilibrium

docker-logs-all:
	@docker-compose logs -f

docker-status:
	@docker-compose ps

# Test Targets
elite-test:
	@echo "========================================================================="
	@echo "ELITE TEST 5/5: Live Docker Stack Integration ROI Proof"
	@echo "========================================================================="
	@echo "Duration: ~25 minutes"
	@echo "Output: ELITE_TEST_5_5_RESULTS.md"
	@echo ""
	@$(PYTHON) elite_test_5_5.py

elite-test-validate:
	@echo "========================================================================="
	@echo "ELITE TEST 5/5: Validation & Execution"
	@echo "========================================================================="
	@echo "This will:"
	@echo "  1. Validate all prerequisites"
	@echo "  2. Check system resources"
	@echo "  3. Verify Docker stack"
	@echo "  4. Run the full test"
	@echo ""
	@$(PYTHON) elite_test_5_5_validate.py

elite-test-quick:
	@echo "Running quick 10-minute validation test..."
	@echo "To run full test, use: make elite-test"
	@echo ""
	@TEST_DURATION_MINUTES=10 $(PYTHON) elite_test_5_5.py

elite-test-results:
	@if [ -f "ELITE_TEST_5_5_RESULTS.md" ]; then \
		cat ELITE_TEST_5_5_RESULTS.md; \
	else \
		echo "No test results found. Run 'make elite-test' first."; \
	fi

elite-test-help:
	@echo "ELITE TEST 5/5 - Available Commands"
	@echo ""
	@echo "  make elite-test            - Run full acquisition-grade ROI test (~25 min)"
	@echo "  make elite-test-validate   - Run with pre-flight checks (~25 min)"
	@echo "  make elite-test-quick      - Run quick validation (10 min)"
	@echo "  make elite-test-results    - Display last test results"
	@echo ""
	@echo "Infrastructure Commands:"
	@echo "  make docker-up             - Start Docker stack"
	@echo "  make docker-down           - Stop Docker stack"
	@echo "  make docker-status         - Show container status"
	@echo "  make docker-logs           - View LoadEquilibrium logs"
	@echo "  make docker-logs-all       - View all container logs"
	@echo ""
	@echo "Usage Examples:"
	@echo "  make docker-up && make elite-test && make docker-down"
	@echo "  make elite-test-validate"
	@echo "  make elite-test-results"
