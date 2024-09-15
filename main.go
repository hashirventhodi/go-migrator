package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Command-line flags
var (
	dbHost      string
	dbPort      int
	dbUser      string
	dbPassword  string
	dbName      string
	outputDir   string
	debug       bool
	versionFlag bool
)

func init() {
	flag.StringVar(&dbHost, "host", "localhost", "Database host")
	flag.IntVar(&dbPort, "port", 5432, "Database port")
	flag.StringVar(&dbUser, "user", "", "Database user")
	flag.StringVar(&dbPassword, "password", "", "Database password")
	flag.StringVar(&dbName, "dbname", "", "Database name")
	flag.StringVar(&outputDir, "output", "migrations", "Output directory for migration files")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.BoolVar(&versionFlag, "version", false, "Prints the version of the CLI")
}

func main() {
	flag.Parse()

	if versionFlag {
		fmt.Println("go-migrator version 0.0.2")
		os.Exit(0)
	}

	if dbUser == "" || dbName == "" {
		log.Fatal("Database user and name are required")
	}

	// Construct the DSN
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	// Connect to the database
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Get the underlying SQL database
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Failed to get database: %v", err)
	}

	// Generate migrations
	generateMigrations(db, sqlDB)
}

// Define your models here
type User struct {
	gorm.Model
	Name  string
	Email string
}

func generateMigrations(db *gorm.DB, sqlDB *sql.DB) {
	models := []interface{}{
		&User{}, // Add all your models here
	}

	for _, model := range models {
		tableName := db.NamingStrategy.TableName(reflect.TypeOf(model).Elem().Name())

		if debug {
			log.Printf("Processing model: %s", reflect.TypeOf(model).Elem().Name())
		}

		var exists bool
		err := sqlDB.QueryRow("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = $1)", tableName).Scan(&exists)
		if err != nil {
			log.Printf("Error checking if table exists: %v", err)
			continue
		}

		if !exists {
			upSQL := generateCreateTableSQL(db, model)
			downSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s;", tableName)
			if upSQL == "" {
				log.Printf("Failed to generate CREATE TABLE SQL for %s", tableName)
				continue
			}
			createMigrationFile(fmt.Sprintf("create_%s_table", tableName), upSQL, true)
			createMigrationFile(fmt.Sprintf("drop_%s_table", tableName), downSQL, false)
		} else {
			differences := compareModelToTable(db, sqlDB, model)
			if len(differences) > 0 {
				upSQL := generateAlterTableSQL(tableName, differences)
				downSQL := generateRollbackAlterTableSQL(tableName, differences)
				if upSQL == "" || downSQL == "" {
					log.Printf("Failed to generate ALTER TABLE SQL for %s", tableName)
					continue
				}
				createMigrationFile(fmt.Sprintf("alter_%s_table", tableName), upSQL, true)
				createMigrationFile(fmt.Sprintf("rollback_%s_table", tableName), downSQL, false)
			} else if debug {
				log.Printf("No differences found for table %s", tableName)
			}
		}
	}
}

func compareModelToTable(db *gorm.DB, sqlDB *sql.DB, model interface{}) []string {
	var differences []string
	modelType := reflect.TypeOf(model).Elem()
	tableName := db.NamingStrategy.TableName(modelType.Name())

	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		columnName := db.NamingStrategy.ColumnName("", field.Name)

		// Skip gorm.Model fields
		if field.Name == "Model" && field.Type == reflect.TypeOf(gorm.Model{}) {
			continue
		}

		var exists bool
		err := sqlDB.QueryRow("SELECT EXISTS (SELECT FROM information_schema.columns WHERE table_name = $1 AND column_name = $2)", tableName, columnName).Scan(&exists)
		if err != nil {
			log.Printf("Error checking if column exists: %v", err)
			continue
		}

		if !exists {
			differences = append(differences, fmt.Sprintf("ADD COLUMN %s %s", columnName, getPostgresType(field.Type)))
		} else {
			var columnType string
			err := sqlDB.QueryRow("SELECT data_type FROM information_schema.columns WHERE table_name = $1 AND column_name = $2", tableName, columnName).Scan(&columnType)
			if err != nil {
				log.Printf("Error getting column type: %v", err)
				continue
			}

			if !strings.EqualFold(columnType, getPostgresType(field.Type)) {
				differences = append(differences, fmt.Sprintf("ALTER COLUMN %s TYPE %s", columnName, getPostgresType(field.Type)))
			}
		}
	}

	return differences
}

func generateAlterTableSQL(tableName string, differences []string) string {
	if len(differences) == 0 {
		return ""
	}
	return fmt.Sprintf("ALTER TABLE %s\n%s;", tableName, strings.Join(differences, ",\n"))
}

func generateRollbackAlterTableSQL(tableName string, differences []string) string {
	var rollbackStatements []string
	for _, diff := range differences {
		if strings.HasPrefix(diff, "ADD COLUMN") {
			columnName := strings.Fields(diff)[2]
			rollbackStatements = append(rollbackStatements, fmt.Sprintf("DROP COLUMN %s", columnName))
		}
	}
	if len(rollbackStatements) == 0 {
		return ""
	}
	return fmt.Sprintf("ALTER TABLE %s\n%s;", tableName, strings.Join(rollbackStatements, ",\n"))
}

func createMigrationFile(name, content string, isUp bool) {
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

	err := os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		log.Fatalf("Failed to create migrations directory: %v", err)
	}

	filepath := filepath.Join(outputDir, filename)
	err = os.WriteFile(filepath, []byte(content), 0644)
	if err != nil {
		log.Fatalf("Failed to write migration file: %v", err)
	}

	fmt.Printf("Created migration file: %s\n", filepath)
	if debug {
		fmt.Printf("Content:\n%s\n", content)
	}
}

func generateCreateTableSQL(db *gorm.DB, model interface{}) string {
	modelType := reflect.TypeOf(model).Elem()
	var columns []string
	var primaryKeys []string
	var foreignKeys []string
	var indexes []string
	var comments []string

	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		columnName := db.NamingStrategy.ColumnName("", field.Name)

		if field.Name == "Model" && field.Type == reflect.TypeOf(gorm.Model{}) {
			columns = append(columns, "id BIGSERIAL PRIMARY KEY")
			columns = append(columns, "created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP")
			columns = append(columns, "updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP")
			columns = append(columns, "deleted_at TIMESTAMP NULL")
			continue
		}

		gormTag := field.Tag.Get("gorm")
		columnDefinition := buildColumnDefinition(columnName, field.Type, gormTag)

		if strings.Contains(gormTag, "primaryKey") {
			primaryKeys = append(primaryKeys, columnName)
		}

		if foreignKey := getForeignKey(gormTag, columnName); foreignKey != "" {
			foreignKeys = append(foreignKeys, foreignKey)
		}

		if strings.Contains(gormTag, "index") {
			indexes = append(indexes, fmt.Sprintf("CREATE INDEX idx_%s ON %s (%s);", columnName, db.NamingStrategy.TableName(modelType.Name()), columnName))
		}

		if comment := getComment(gormTag); comment != "" {
			comments = append(comments, fmt.Sprintf("COMMENT ON COLUMN %s.%s IS '%s';", db.NamingStrategy.TableName(modelType.Name()), columnName, comment))
		}

		columns = append(columns, columnDefinition)
	}

	if len(primaryKeys) > 0 {
		columns = append(columns, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(primaryKeys, ", ")))
	}

	if len(columns) == 0 {
		return ""
	}

	return fmt.Sprintf("CREATE TABLE %s (\n%s\n);", db.NamingStrategy.TableName(modelType.Name()), strings.Join(columns, ",\n"))
}

func getPostgresType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "VARCHAR"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "BIGINT"
	case reflect.Float32, reflect.Float64:
		return "DOUBLE PRECISION"
	case reflect.Bool:
		return "BOOLEAN"
	case reflect.Struct:
		if t.Name() == "Time" {
			return "TIMESTAMP"
		}
	}
	return "TEXT"
}

func buildColumnDefinition(columnName string, fieldType reflect.Type, gormTag string) string {
	postgresType := getPostgresType(fieldType)
	columnDef := fmt.Sprintf("%s %s", columnName, postgresType)

	if strings.Contains(gormTag, "not null") {
		columnDef += " NOT NULL"
	}
	if strings.Contains(gormTag, "unique") {
		columnDef += " UNIQUE"
	}
	if strings.Contains(gormTag, "default") {
		columnDef += fmt.Sprintf(" DEFAULT %s", getDefault(gormTag))
	}

	return columnDef
}

func getForeignKey(gormTag, columnName string) string {
	if strings.Contains(gormTag, "foreignKey") {
		parts := strings.Split(gormTag, ":")
		return fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s", columnName, parts[1])
	}
	return ""
}

func getComment(gormTag string) string {
	if strings.Contains(gormTag, "comment") {
		parts := strings.Split(gormTag, ":")
		return parts[1]
	}
	return ""
}

func getDefault(gormTag string) string {
	parts := strings.Split(gormTag, ":")
	return parts[1]
}
