// File: migrator/migrator.go

package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Config struct {
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
	OutputDir  string
	Debug      bool
}

type Migrator struct {
	config Config
	db     *gorm.DB
	sqlDB  *sql.DB
	models []interface{}
}

func New(config Config) (*Migrator, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		config.DBHost, config.DBPort, config.DBUser, config.DBPassword, config.DBName)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database: %v", err)
	}

	return &Migrator{
		config: config,
		db:     db,
		sqlDB:  sqlDB,
		models: []interface{}{},
	}, nil
}

func (m *Migrator) AddModel(model interface{}) {
	m.models = append(m.models, model)
}

func (m *Migrator) GenerateMigrations() error {
	for _, model := range m.models {
		tableName := m.db.NamingStrategy.TableName(reflect.TypeOf(model).Elem().Name())

		if m.config.Debug {
			log.Printf("Processing model: %s", reflect.TypeOf(model).Elem().Name())
		}

		// Check if table exists
		var exists bool
		err := m.sqlDB.QueryRow("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = $1)", tableName).Scan(&exists)
		if err != nil {
			log.Printf("Error checking if table exists: %v", err)
			continue
		}

		if !exists {
			// Table doesn't exist, create a new migration to create the table
			upSQL := m.generateCreateTableSQL(model)
			downSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s;", tableName)
			if upSQL == "" {
				log.Printf("Failed to generate CREATE TABLE SQL for %s", tableName)
				continue
			}
			m.createMigrationFile(fmt.Sprintf("create_%s_table", tableName), upSQL, true)
			m.createMigrationFile(fmt.Sprintf("drop_%s_table", tableName), downSQL, false)
		} else {
			// Table exists, check for differences and create migration if needed
			differences := m.compareModelToTable(model)
			if len(differences) > 0 {
				upSQL := m.generateAlterTableSQL(tableName, differences)
				downSQL := m.generateRollbackAlterTableSQL(tableName, differences)
				if upSQL == "" || downSQL == "" {
					log.Printf("Failed to generate ALTER TABLE SQL for %s", tableName)
					continue
				}
				m.createMigrationFile(fmt.Sprintf("alter_%s_table", tableName), upSQL, true)
				m.createMigrationFile(fmt.Sprintf("rollback_%s_table", tableName), downSQL, false)
			} else if m.config.Debug {
				log.Printf("No differences found for table %s", tableName)
			}
		}
	}
	return nil
}

func (m *Migrator) createMigrationFile(name, content string, isUp bool) {
	if content == "" {
		log.Printf("Skipping creation of empty migration file: %s", name)
		return
	}

	timestamp := time.Now().Format("20060102150405")
	direction := "up"
	if !isUp {
		direction = "down"
	}
	filename := fmt.Sprintf("%s_%s.%s.sql", timestamp, name, direction)

	err := os.MkdirAll(m.config.OutputDir, os.ModePerm)
	if err != nil {
		log.Fatalf("Failed to create migrations directory: %v", err)
	}

	filepath := filepath.Join(m.config.OutputDir, filename)
	err = os.WriteFile(filepath, []byte(content), 0644)
	if err != nil {
		log.Fatalf("Failed to write migration file: %v", err)
	}

	fmt.Printf("Created migration file: %s\n", filepath)
	if m.config.Debug {
		fmt.Printf("Content:\n%s\n", content)
	}
}
