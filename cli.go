// File: migrator/cli.go

package migrator

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	dbHost1     string
	dbPort1     int
	dbUser1     string
	dbPassword1 string
	dbName1     string
	outputDir1  string
	debug1      bool
)

var rootCmd = &cobra.Command{
	Use:   "migrator",
	Short: "Migrator is a tool for generating database migrations",
	Long: `Migrator is a CLI tool that generates database migrations
based on your GORM models. It compares your models with the
current database schema and creates migration files for any
differences it finds.`,
}

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate migration files",
	Long:  `Generate migration files based on the differences between your models and the current database schema.`,
	Run: func(cmd *cobra.Command, args []string) {
		config := Config{
			DBHost:     dbHost,
			DBPort:     dbPort,
			DBUser:     dbUser,
			DBPassword: dbPassword,
			DBName:     dbName,
			OutputDir:  outputDir,
			Debug:      debug,
		}

		m, err := New(config)
		if err != nil {
			fmt.Printf("Failed to create migrator: %v\n", err)
			os.Exit(1)
		}

		err = m.GenerateMigrations()
		if err != nil {
			fmt.Printf("Failed to generate migrations: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Migrations generated successfully!")
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)

	generateCmd.Flags().StringVar(&dbHost, "host", "localhost", "Database host")
	generateCmd.Flags().IntVar(&dbPort, "port", 5432, "Database port")
	generateCmd.Flags().StringVar(&dbUser, "user", "", "Database user")
	generateCmd.Flags().StringVar(&dbPassword, "password", "", "Database password")
	generateCmd.Flags().StringVar(&dbName, "dbname", "", "Database name")
	generateCmd.Flags().StringVar(&outputDir, "output", "migrations", "Output directory for migration files")
	generateCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug mode")

	generateCmd.MarkFlagRequired("user")
	generateCmd.MarkFlagRequired("dbname")
}

// RunCLI starts the CLI application
func RunCLI() error {
	return rootCmd.Execute()
}
