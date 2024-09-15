// File: migrator/sql_helpers.go

package main

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"gorm.io/gorm"
)

func (m *Migrator) getPostgresType(goType reflect.Type) string {
	switch goType.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "BIGINT"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "BIGINT"
	case reflect.String:
		return "TEXT"
	case reflect.Bool:
		return "BOOLEAN"
	case reflect.Float32, reflect.Float64:
		return "FLOAT"
	case reflect.Struct:
		if goType == reflect.TypeOf(time.Time{}) {
			return "TIMESTAMP"
		}
	}
	return "TEXT"
}

func (m *Migrator) buildColumnDefinition(columnName string, goType reflect.Type, gormTag string) string {
	columnType := m.getPostgresType(goType)
	// Default column definition
	columnDef := fmt.Sprintf("%s %s", columnName, columnType)

	// Handle optional constraints based on GORM tag
	if strings.Contains(gormTag, "not null") {
		columnDef += " NOT NULL"
	}
	if defaultValue := m.getDefault(gormTag); defaultValue != "" {
		columnDef += fmt.Sprintf(" DEFAULT %s", defaultValue)
	}
	return columnDef
}

func (m *Migrator) getForeignKey(gormTag, columnName string) string {
	// Parse GORM tag for foreign key information
	if strings.Contains(gormTag, "foreignKey") {
		// Example GORM tag: `gorm:"foreignKey:UserID;references:ID"`
		parts := strings.Split(gormTag, ";")
		for _, part := range parts {
			if strings.HasPrefix(part, "foreignKey:") {
				references := strings.Split(part, ":")[1]
				return fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s(id)", columnName, references)
			}
		}
	}
	return ""
}

func (m *Migrator) getComment(gormTag string) string {
	// Extract comment from GORM tag if present
	if strings.Contains(gormTag, "comment:") {
		return strings.Split(gormTag, ":")[1]
	}
	return ""
}

func (m *Migrator) getDefault(gormTag string) string {
	// Extract default value from GORM tag if present
	if strings.Contains(gormTag, "default:") {
		return strings.Split(gormTag, ":")[1]
	}
	return ""
}

func (m *Migrator) generateCreateTableSQL(model interface{}) string {
	modelType := reflect.TypeOf(model).Elem()
	var columns []string
	var primaryKeys []string
	var foreignKeys []string
	var indexes []string
	var comments []string

	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		columnName := m.db.NamingStrategy.ColumnName("", field.Name)

		// Handle `gorm.Model` separately (ID, CreatedAt, UpdatedAt, DeletedAt)
		if field.Name == "Model" && field.Type == reflect.TypeOf(gorm.Model{}) {
			columns = append(columns, "id BIGSERIAL PRIMARY KEY")
			columns = append(columns, "created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP")
			columns = append(columns, "updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP")
			columns = append(columns, "deleted_at TIMESTAMP NULL")
			continue
		}

		// Extract GORM struct tags (type, primary key, unique, not null, default, etc.)
		gormTag := field.Tag.Get("gorm")
		columnDefinition := m.buildColumnDefinition(columnName, field.Type, gormTag)

		// Handle primary key if defined in GORM tag
		if strings.Contains(gormTag, "primaryKey") {
			primaryKeys = append(primaryKeys, columnName)
		}

		// Handle foreign key constraints
		if foreignKey := m.getForeignKey(gormTag, columnName); foreignKey != "" {
			foreignKeys = append(foreignKeys, foreignKey)
		}

		// Handle index creation
		if strings.Contains(gormTag, "index") {
			indexes = append(indexes, fmt.Sprintf("CREATE INDEX idx_%s ON %s (%s);", columnName, m.db.NamingStrategy.TableName(modelType.Name()), columnName))
		}

		// Handle comments if present
		if comment := m.getComment(gormTag); comment != "" {
			comments = append(comments, fmt.Sprintf("COMMENT ON COLUMN %s.%s IS '%s';", m.db.NamingStrategy.TableName(modelType.Name()), columnName, comment))
		}

		columns = append(columns, columnDefinition)
	}

	// Build the SQL string
	sql := fmt.Sprintf("CREATE TABLE %s (\n%s", m.db.NamingStrategy.TableName(modelType.Name()), strings.Join(columns, ",\n"))

	if len(primaryKeys) > 0 {
		sql += fmt.Sprintf(",\nPRIMARY KEY (%s)", strings.Join(primaryKeys, ", "))
	}

	sql += "\n);"

	// Add foreign key constraints
	if len(foreignKeys) > 0 {
		sql += "\n" + strings.Join(foreignKeys, "\n") + "\n"
	}

	// Add comments
	if len(comments) > 0 {
		sql += "\n" + strings.Join(comments, "\n") + "\n"
	}

	return sql
}
