// Package models holds all GORM domain entities, the single source of truth for
// the persistence schema. Models are split by domain across files in this package:
// user.go, project.go, board.go, task.go, team.go, attendance.go, report.go,
// problem.go, subscription.go, apitoken.go, llm.go, conveyor.go, git.go.
//
// Each model declares an explicit TableName() (in its own file) to keep GORM's
// schema cache deterministic.
//
// Sessions are stored in Redis and intentionally have no GORM model here.
package models
