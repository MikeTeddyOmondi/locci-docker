package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/MikeTeddyOmondi/locci-docker/internal/config"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

// userCmd manages users directly in the server's database (run it on the server host).
func userCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage users in the local database (run on the server host)",
	}

	var admin bool
	add := &cobra.Command{
		Use:   "add <username>",
		Short: "Create a user (prompts for the password)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(db *store.Store) error {
				role := "user"
				if admin {
					role = "admin"
				}
				u, err := db.CreateUser(args[0], readPassword(), role)
				if err != nil {
					return err
				}
				fmt.Printf("created %s (%s)\n", u.Username, u.Role)
				return nil
			})
		},
	}
	add.Flags().BoolVar(&admin, "admin", false, "give the user admin rights")

	ls := &cobra.Command{
		Use:   "ls",
		Short: "List users",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(db *store.Store) error {
				users, err := db.ListUsers()
				if err != nil {
					return err
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
				fmt.Fprintln(w, "USERNAME\tROLE\tCREATED")
				for _, u := range users {
					fmt.Fprintf(w, "%s\t%s\t%s\n", u.Username, u.Role, u.Created)
				}
				return w.Flush()
			})
		},
	}

	rm := &cobra.Command{
		Use:   "rm <username>",
		Short: "Delete a user and their tokens",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(db *store.Store) error { return db.DeleteUser(args[0]) })
		},
	}

	passwd := &cobra.Command{
		Use:   "passwd <username>",
		Short: "Change a user's password",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(db *store.Store) error { return db.SetPassword(args[0], readPassword()) })
		},
	}

	cmd.AddCommand(add, ls, rm, passwd)
	return cmd
}

func withStore(fn func(*store.Store) error) error {
	db, err := store.Open(config.Load().DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	return fn(db)
}

// readPassword reads LOCCI_PASSWORD or one line from stdin.
func readPassword() string {
	if p := os.Getenv("LOCCI_PASSWORD"); p != "" {
		return p
	}
	fmt.Fprint(os.Stderr, "password: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line)
}
