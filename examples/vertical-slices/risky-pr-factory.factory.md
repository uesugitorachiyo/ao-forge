# Objective
Run a governed risky PR improvement against the fixture repository.

# Workspace
fixtures/discount-service

# Constraints
- Local First: true
- Allow Network: false
- Allow Release Mutation: false
- Require Control Plane Readback: false
- Release Mode: false

# Expected Workcells
- prepare-fixture (prepare)
- run-ao2-risky-pr (execute) depends on: prepare-fixture
- close-factory-packet (close) depends on: run-ao2-risky-pr

# Expected Evidence
- ao2 run summary
- covenant policy decision
- factory packet
