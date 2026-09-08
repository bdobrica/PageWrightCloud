package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/auth"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/config"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	_ "github.com/lib/pq"
)

const (
	exitSuccess = 0
	exitError   = 1
)

func main() {
	os.Exit(run())
}

func run() int {
	// Define subcommands
	createCmd := flag.NewFlagSet("create", flag.ExitOnError)
	createEmail := createCmd.String("email", "", "User email (required)")
	createPassword := createCmd.String("password", "", "User password (required)")
	passwordStdin := createCmd.Bool("password-stdin", false, "Read password from stdin instead of a process argument")

	listCmd := flag.NewFlagSet("list", flag.ExitOnError)

	deleteCmd := flag.NewFlagSet("delete", flag.ExitOnError)
	deleteEmail := deleteCmd.String("email", "", "User email to delete (required)")

	// Show usage if no command provided
	if len(os.Args) < 2 {
		printUsage()
		return exitError
	}

	// Load configuration from environment
	cfg := config.LoadConfig()
	if err := config.ValidateDatabase(cfg.DatabaseURL); err != nil {
		log.Print(err)
		return exitError
	}

	// Connect to database
	db, err := database.NewDB(cfg.DatabaseURL)
	if err != nil {
		log.Print("Error: Failed to connect to database; verify PostgreSQL configuration and readiness")
		return exitError
	}
	defer db.Close()

	// Parse subcommand
	switch os.Args[1] {
	case "create":
		createCmd.Parse(os.Args[2:])
		if *passwordStdin {
			if *createPassword != "" {
				log.Println("Use only one password input method")
				return exitError
			}
			raw, err := io.ReadAll(io.LimitReader(os.Stdin, 74))
			if err != nil {
				log.Println("Cannot read password")
				return exitError
			}
			*createPassword = strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
		}
		if *createEmail == "" || *createPassword == "" {
			log.Println("Error: Both -email and -password flags are required")
			createCmd.PrintDefaults()
			return exitError
		}
		return createUser(db, *createEmail, *createPassword)

	case "list":
		listCmd.Parse(os.Args[2:])
		return listUsers(db)

	case "delete":
		deleteCmd.Parse(os.Args[2:])
		if *deleteEmail == "" {
			log.Println("Error: -email flag is required")
			deleteCmd.PrintDefaults()
			return exitError
		}
		return deleteUser(db, *deleteEmail)

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		return exitError
	}
}

func printUsage() {
	fmt.Println("PageWright User Management CLI")
	fmt.Println("\nUsage:")
	fmt.Println("  users <command> [options]")
	fmt.Println("\nCommands:")
	fmt.Println("  create      Create a new user")
	fmt.Println("    -email string      User email (required)")
	fmt.Println("    -password string   User password (required)")
	fmt.Println("    -password-stdin    Read password from stdin (preferred)")
	fmt.Println("\n  list        List all users")
	fmt.Println("\n  delete      Delete a user")
	fmt.Println("    -email string      User email to delete (required)")
	fmt.Println("\nEnvironment Variables:")
	fmt.Println("  PAGEWRIGHT_DATABASE_URL    Direct deployment URL, or Compose-supplied database fields")
	fmt.Println("\nExamples:")
	fmt.Println("  users create -email admin@example.com -password-stdin")
	fmt.Println("  users list")
	fmt.Println("  users delete -email admin@example.com")
}

func createUser(db *database.DB, email, password string) int {
	if err := auth.ValidatePassword(password); err != nil {
		log.Print(err)
		return exitError
	}
	// Check if user already exists
	existingUser, err := db.GetUserByEmail(email)
	if err != nil {
		log.Print("Error: Failed to check account existence; database diagnostics withheld")
		return exitError
	}
	if existingUser != nil {
		log.Printf("Error: User with email %s already exists\n", email)
		return exitError
	}

	// Hash the password
	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		log.Print("Error: Failed to hash password")
		return exitError
	}

	// Create the user
	user, err := db.CreateUser(email, hashedPassword, nil, nil)
	if err != nil {
		log.Print("Error: Failed to create user; database diagnostics withheld")
		return exitError
	}

	fmt.Printf("✓ User created successfully\n")
	fmt.Printf("  ID:    %s\n", user.ID)
	fmt.Printf("  Email: %s\n", user.Email)
	fmt.Printf("  Created: %s\n", user.CreatedAt.Format("2006-01-02 15:04:05"))

	return exitSuccess
}

func listUsers(db *database.DB) int {
	users, err := db.ListUsers()
	if err != nil {
		log.Print("Error: Failed to list users; database diagnostics withheld")
		return exitError
	}

	if len(users) == 0 {
		fmt.Println("No users found")
		return exitSuccess
	}

	// Create a tabwriter for formatted output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tEMAIL\tAUTH METHOD\tCREATED AT")
	fmt.Fprintln(w, "----\t-----\t-----------\t----------")

	for _, user := range users {
		authMethod := "password"
		if user.OAuthProvider != nil {
			authMethod = fmt.Sprintf("oauth (%s)", *user.OAuthProvider)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			user.ID,
			user.Email,
			authMethod,
			user.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	w.Flush()

	fmt.Printf("\nTotal: %d user(s)\n", len(users))
	return exitSuccess
}

func deleteUser(db *database.DB, email string) int {
	// Check if user exists
	user, err := db.GetUserByEmail(email)
	if err != nil {
		log.Print("Error: Failed to check account existence; database diagnostics withheld")
		return exitError
	}
	if user == nil {
		log.Printf("Error: User with email %s not found\n", email)
		return exitError
	}

	// Delete the user
	err = db.DeleteUser(user.ID)
	if err != nil {
		log.Print("Error: Failed to delete user; database diagnostics withheld")
		return exitError
	}

	fmt.Printf("✓ User deleted successfully\n")
	fmt.Printf("  Email: %s\n", email)

	return exitSuccess
}
