// File: migrator/model_comparison.go

package main

import (
	"fmt"
	"log"
	"reflect"
	"strings"

	"gorm.io/gorm"
)

func (m *Migrator) compareModelToTable(model interface{}) []string {
	var differences []string
	modelType := reflect.TypeOf(model).Elem()
	tableName := m.db.NamingStrategy.TableName(modelType.Name())

	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		columnName := m.db.NamingStrategy.ColumnName("", field.Name)

		// Skip gorm.Model fields
		if field.Name == "Model" && field.Type == reflect.TypeOf(gorm.Model{}) {
			continue
		}

		// Check if column exists
		var exists bool
		err := m.sqlDB.QueryRow("SELECT EXISTS (SELECT FROM information_schema.columns WHERE table_name = $1 AND column_name = $2)", tableName, columnName).Scan(&exists)
		if err != nil {
			log.Printf("Error checking if column exists: %v", err)
			continue
		}

		if !exists {
			differences = append(differences, fmt.Sprintf("ADD COLUMN %s %s", columnName, m.getPostgresType(field.Type)))
		} else {
			// Check column type
			var columnType string
			err := m.sqlDB.QueryRow("SELECT data_type FROM information_schema.columns WHERE table_name = $1 AND column_name = $2", tableName, columnName).Scan(&columnType)
			if err != nil {
				log.Printf("Error getting column type: %v", err)
				continue
			}

			if !strings.EqualFold(columnType, m.getPostgresType(field.Type)) {
				differences = append(differences, fmt.Sprintf("ALTER COLUMN %s TYPE %s", columnName, m.getPostgresType(field.Type)))
			}
		}
	}

	return differences
}

func (m *Migrator) generateAlterTableSQL(tableName string, differences []string) string {
	if len(differences) == 0 {
		return ""
	}
	return fmt.Sprintf("ALTER TABLE %s\n%s;", tableName, strings.Join(differences, ",\n"))
}

func (m *Migrator) generateRollbackAlterTableSQL(tableName string, differences []string) string {
	var rollbackStatements []string
	for _, diff := range differences {
		if strings.HasPrefix(diff, "ADD COLUMN") {
			columnName := strings.Fields(diff)[2]
			rollbackStatements = append(rollbackStatements, fmt.Sprintf("DROP COLUMN %s", columnName))
		}
		// Add more cases for other types of alterations as needed
	}
	if len(rollbackStatements) == 0 {
		return ""
	}
	return fmt.Sprintf("ALTER TABLE %s\n%s;", tableName, strings.Join(rollbackStatements, ",\n"))
}
