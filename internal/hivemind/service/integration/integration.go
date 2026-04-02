// Package integration provides cross-bounded-context adapters and module assembly.
// This layer is the ONLY place where Team BC, Execution BC, and Collaboration BC
// are directly coupled. All other packages maintain strict module boundaries.
//
// Architecture:
//   - ExecutionPort interface: defined in team package, implemented by teamExecutionAdapter
//   - WorkerIndex: maintains WorkerRef ↔ SubAgentRecordID bidirectional mapping
//   - Adapters: translate between domain models without forcing the other BC to know details
//
// This package is NOT imported by domain BCs — it only imports from them.
// It serves as the "glue layer" assembled by the application bootstrapper.
package integration
