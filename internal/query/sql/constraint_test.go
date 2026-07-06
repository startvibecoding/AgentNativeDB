package sql

import (
	"context"
	"strings"
	"testing"
)

func execSQLForConstraint(t *testing.T, e *Executor, query string) error {
	t.Helper()
	stmt, err := Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	plan, err := e.Planner().Plan(stmt)
	if err != nil {
		t.Fatalf("plan %q: %v", query, err)
	}
	_, err = e.Execute(context.Background(), plan)
	return err
}

func requireConstraintError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error containing %q, got %q", want, err.Error())
	}
}

func TestConstraints_InsertRejectsNotNullViolation(t *testing.T) {
	e := setupIndexExecutor(t)
	runSQL(t, e, "CREATE TABLE users (id VARCHAR(64) PRIMARY KEY, name VARCHAR(64) NOT NULL)")

	err := execSQLForConstraint(t, e, "INSERT INTO users (id) VALUES ('u1')")
	requireConstraintError(t, err, "NOT NULL 约束失败: users.name")
}

func TestConstraints_InsertAppliesDefaultValue(t *testing.T) {
	e := setupIndexExecutor(t)
	runSQL(t, e, "CREATE TABLE users (id VARCHAR(64) PRIMARY KEY, name VARCHAR(64), active BOOL DEFAULT TRUE)")

	runSQL(t, e, "INSERT INTO users (id, name) VALUES ('u1', 'alice')")

	res := runSQL(t, e, "SELECT active FROM users WHERE id = 'u1'")
	if len(res.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(res.Rows))
	}
	if got := res.Rows[0].Values["active"]; got != true {
		t.Fatalf("expected default active=true, got %v", got)
	}
}

func TestConstraints_InsertRejectsPrimaryKeyDuplicate(t *testing.T) {
	e := setupIndexExecutor(t)
	runSQL(t, e, "CREATE TABLE users (id VARCHAR(64) PRIMARY KEY, name VARCHAR(64))")
	runSQL(t, e, "INSERT INTO users (id, name) VALUES ('u1', 'alice')")

	err := execSQLForConstraint(t, e, "INSERT INTO users (id, name) VALUES ('u1', 'bob')")
	requireConstraintError(t, err, "PRIMARY KEY 约束失败: users.id")
}

func TestConstraints_InsertRejectsTypeAndLengthViolations(t *testing.T) {
	e := setupIndexExecutor(t)
	runSQL(t, e, "CREATE TABLE users (id VARCHAR(64) PRIMARY KEY, name VARCHAR(3), age INT)")

	err := execSQLForConstraint(t, e, "INSERT INTO users (id, name, age) VALUES ('u1', 'alice', 30)")
	requireConstraintError(t, err, "VARCHAR 长度约束失败: users.name")

	err = execSQLForConstraint(t, e, "INSERT INTO users (id, name, age) VALUES ('u2', 'bob', 'old')")
	requireConstraintError(t, err, "类型约束失败: users.age 需要 INT")
}

func TestConstraints_UpdateRejectsViolations(t *testing.T) {
	newUsers := func(t *testing.T) *Executor {
		t.Helper()
		e := setupIndexExecutor(t)
		runSQL(t, e, "CREATE TABLE users (id VARCHAR(64) PRIMARY KEY, name VARCHAR(64) NOT NULL, age INT)")
		runSQL(t, e, "INSERT INTO users (id, name, age) VALUES ('u1', 'alice', 30)")
		runSQL(t, e, "INSERT INTO users (id, name, age) VALUES ('u2', 'bob', 25)")
		return e
	}

	e := newUsers(t)
	err := execSQLForConstraint(t, e, "UPDATE users SET name = NULL WHERE id = 'u1'")
	requireConstraintError(t, err, "NOT NULL 约束失败: users.name")

	e = newUsers(t)
	err = execSQLForConstraint(t, e, "UPDATE users SET age = 'old' WHERE id = 'u1'")
	requireConstraintError(t, err, "类型约束失败: users.age 需要 INT")

	e = newUsers(t)
	err = execSQLForConstraint(t, e, "UPDATE users SET id = 'u2' WHERE id = 'u1'")
	requireConstraintError(t, err, "PRIMARY KEY 约束失败: users.id")
}

func TestConstraints_UpdatePrimaryKeyMovesRow(t *testing.T) {
	e := setupIndexExecutor(t)
	runSQL(t, e, "CREATE TABLE users (id VARCHAR(64) PRIMARY KEY, name VARCHAR(64))")
	runSQL(t, e, "INSERT INTO users (id, name) VALUES ('u1', 'alice')")

	runSQL(t, e, "UPDATE users SET id = 'u2' WHERE id = 'u1'")

	oldRes := runSQL(t, e, "SELECT * FROM users WHERE id = 'u1'")
	if len(oldRes.Rows) != 0 {
		t.Fatalf("expected old primary key row to be removed, got %d rows", len(oldRes.Rows))
	}
	newRes := runSQL(t, e, "SELECT * FROM users WHERE id = 'u2'")
	if len(newRes.Rows) != 1 {
		t.Fatalf("expected new primary key row, got %d rows", len(newRes.Rows))
	}
	if newRes.Rows[0].Values["name"] != "alice" {
		t.Fatalf("unexpected row: %#v", newRes.Rows[0].Values)
	}
}

func TestConstraints_InsertWithoutColumnListUsesTableOrder(t *testing.T) {
	e := setupIndexExecutor(t)
	runSQL(t, e, "CREATE TABLE users (id VARCHAR(64) PRIMARY KEY, name VARCHAR(64) NOT NULL, age INT)")

	runSQL(t, e, "INSERT INTO users VALUES ('u1', 'alice', 30)")

	res := runSQL(t, e, "SELECT name, age FROM users WHERE id = 'u1'")
	if len(res.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(res.Rows))
	}
	if res.Rows[0].Values["name"] != "alice" || toFloat(res.Rows[0].Values["age"]) != 30 {
		t.Fatalf("unexpected row: %#v", res.Rows[0].Values)
	}
}
