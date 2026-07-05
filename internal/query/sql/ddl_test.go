package sql

import (
	"testing"
)

func TestParseCreateTable(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "simple create table",
			input:   "CREATE TABLE users (id INT PRIMARY KEY, name VARCHAR(100), email VARCHAR(255))",
			wantErr: false,
		},
		{
			name:    "create table with if not exists",
			input:   "CREATE TABLE IF NOT EXISTS users (id INT PRIMARY KEY, name VARCHAR(100))",
			wantErr: false,
		},
		{
			name:    "create table with various types",
			input:   "CREATE TABLE products (id INT PRIMARY KEY, name VARCHAR(255) NOT NULL, price FLOAT, description TEXT, active BOOL DEFAULT TRUE)",
			wantErr: false,
		},
		{
			name:    "create table with nullable columns",
			input:   "CREATE TABLE orders (id INT PRIMARY KEY, user_id INT NOT NULL, note TEXT)",
			wantErr: false,
		},
		{
			name:    "create table with STRING type",
			input:   "CREATE TABLE aaa2a (id STRING PRIMARY KEY, aa2a STRING)",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmt, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if _, ok := stmt.(*CreateTableStmt); !ok {
					t.Errorf("Parse() expected CreateTableStmt, got %T", stmt)
				}
			}
		})
	}
}

func TestParseDropTable(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "simple drop table",
			input:   "DROP TABLE users",
			wantErr: false,
		},
		{
			name:    "drop table if exists",
			input:   "DROP TABLE IF EXISTS users",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmt, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if _, ok := stmt.(*DropTableStmt); !ok {
					t.Errorf("Parse() expected DropTableStmt, got %T", stmt)
				}
			}
		})
	}
}

func TestParseAlterTable(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "add column",
			input:   "ALTER TABLE users ADD COLUMN age INT",
			wantErr: false,
		},
		{
			name:    "add column without COLUMN keyword",
			input:   "ALTER TABLE users ADD age INT",
			wantErr: false,
		},
		{
			name:    "drop column",
			input:   "ALTER TABLE users DROP COLUMN age",
			wantErr: false,
		},
		{
			name:    "drop column without COLUMN keyword",
			input:   "ALTER TABLE users DROP age",
			wantErr: false,
		},
		{
			name:    "modify column",
			input:   "ALTER TABLE users MODIFY COLUMN name VARCHAR(200) NOT NULL",
			wantErr: false,
		},
		{
			name:    "modify column without COLUMN keyword",
			input:   "ALTER TABLE users MODIFY name VARCHAR(200) NOT NULL",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmt, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if _, ok := stmt.(*AlterTableStmt); !ok {
					t.Errorf("Parse() expected AlterTableStmt, got %T", stmt)
				}
			}
		})
	}
}

func TestParseShowTables(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "show tables",
			input:   "SHOW TABLES",
			wantErr: false,
		},
		{
			name:    "show tables with semicolon",
			input:   "SHOW TABLES;",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmt, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if _, ok := stmt.(*ShowTablesStmt); !ok {
					t.Errorf("Parse() expected ShowTablesStmt, got %T", stmt)
				}
			}
		})
	}
}

func TestParseDescribeTable(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "describe table",
			input:   "DESCRIBE users",
			wantErr: false,
		},
		{
			name:    "desc table",
			input:   "DESC users",
			wantErr: false,
		},
		{
			name:    "describe table with semicolon",
			input:   "DESCRIBE users;",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmt, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if _, ok := stmt.(*DescribeTableStmt); !ok {
					t.Errorf("Parse() expected DescribeTableStmt, got %T", stmt)
				}
			}
		})
	}
}

func TestCreateTableAST(t *testing.T) {
	input := "CREATE TABLE IF NOT EXISTS users (id INT PRIMARY KEY, name VARCHAR(100) NOT NULL, email VARCHAR(255), age INT DEFAULT 0)"
	stmt, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	createStmt, ok := stmt.(*CreateTableStmt)
	if !ok {
		t.Fatalf("Expected CreateTableStmt, got %T", stmt)
	}

	if !createStmt.IfNotExists {
		t.Error("Expected IfNotExists to be true")
	}

	if createStmt.Table != "users" {
		t.Errorf("Expected table name 'users', got '%s'", createStmt.Table)
	}

	if len(createStmt.Columns) != 4 {
		t.Fatalf("Expected 4 columns, got %d", len(createStmt.Columns))
	}

	// 检查 id 列
	idCol := createStmt.Columns[0]
	if idCol.Name != "id" {
		t.Errorf("Expected column name 'id', got '%s'", idCol.Name)
	}
	if idCol.Type.Name != "INT" {
		t.Errorf("Expected column type 'INT', got '%s'", idCol.Type.Name)
	}
	if !idCol.PrimaryKey {
		t.Error("Expected id column to be primary key")
	}
	if idCol.Nullable {
		t.Error("Expected id column to be not nullable")
	}

	// 检查 name 列
	nameCol := createStmt.Columns[1]
	if nameCol.Name != "name" {
		t.Errorf("Expected column name 'name', got '%s'", nameCol.Name)
	}
	if nameCol.Type.Name != "VARCHAR" {
		t.Errorf("Expected column type 'VARCHAR', got '%s'", nameCol.Type.Name)
	}
	if nameCol.Type.Length != 100 {
		t.Errorf("Expected column length 100, got %d", nameCol.Type.Length)
	}
	if nameCol.Nullable {
		t.Error("Expected name column to be not nullable")
	}

	// 检查 age 列
	ageCol := createStmt.Columns[3]
	if ageCol.Default == nil {
		t.Error("Expected age column to have default value")
	}
}

func TestAlterTableAST(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(*AlterTableStmt) bool
	}{
		{
			name:  "add column",
			input: "ALTER TABLE users ADD COLUMN age INT",
			check: func(stmt *AlterTableStmt) bool {
				if stmt.Table != "users" {
					return false
				}
			 addAction, ok := stmt.Action.(*AddColumnAction)
				if !ok {
					return false
				}
				return addAction.Column.Name == "age" && addAction.Column.Type.Name == "INT"
			},
		},
		{
			name:  "drop column",
			input: "ALTER TABLE users DROP COLUMN age",
			check: func(stmt *AlterTableStmt) bool {
				if stmt.Table != "users" {
					return false
				}
				dropAction, ok := stmt.Action.(*DropColumnAction)
				if !ok {
					return false
				}
				return dropAction.Column == "age"
			},
		},
		{
			name:  "modify column",
			input: "ALTER TABLE users MODIFY COLUMN name VARCHAR(200) NOT NULL",
			check: func(stmt *AlterTableStmt) bool {
				if stmt.Table != "users" {
					return false
				}
				modifyAction, ok := stmt.Action.(*ModifyColumnAction)
				if !ok {
					return false
				}
				return modifyAction.Column.Name == "name" &&
					modifyAction.Column.Type.Name == "VARCHAR" &&
					modifyAction.Column.Type.Length == 200 &&
					!modifyAction.Column.Nullable
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmt, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			alterStmt, ok := stmt.(*AlterTableStmt)
			if !ok {
				t.Fatalf("Expected AlterTableStmt, got %T", stmt)
			}

			if !tt.check(alterStmt) {
				t.Error("AST check failed")
			}
		})
	}
}
