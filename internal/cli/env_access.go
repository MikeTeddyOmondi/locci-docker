package cli

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// envAccess is who may use an environment, as the API reports it.
type envAccess struct {
	Restricted bool     `json:"restricted"`
	Users      []string `json:"users"`
}

func envAccessCmd() *cobra.Command {
	var users []string
	var adminsOnly, everyone bool
	cmd := &cobra.Command{
		Use:   "access NAME",
		Short: "Show or set who may use an environment (admin)",
		Long: `Without flags, show who may use NAME. An environment is open to every
signed-in user until restricted; admins can always use it.

  locci env access prod --users ann,bob   only ann and bob (and admins)
  locci env access prod --admins-only     only admins
  locci env access prod --everyone        every signed-in user again`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/api/v1/environments/" + url.PathEscape(args[0])
			n := 0
			for _, set := range []bool{len(users) > 0, adminsOnly, everyone} {
				if set {
					n++
				}
			}

			var access envAccess
			switch {
			case n > 1:
				return errors.New("use one of --users, --admins-only and --everyone")
			case n == 1:
				body := map[string]any{"restricted": !everyone, "users": users}
				if err := api().Do("PUT", path+"/access", body, &access); err != nil {
					return err
				}
			default:
				var env struct {
					Access *envAccess `json:"access"`
				}
				if err := api().Do("GET", path, nil, &env); err != nil {
					return err
				}
				if env.Access == nil {
					return errors.New("only admins can see who may use an environment")
				}
				access = *env.Access
			}

			if outputJSON {
				return printJSON(access)
			}
			switch {
			case !access.Restricted:
				fmt.Println(args[0] + ": every signed-in user")
			case len(access.Users) == 0:
				fmt.Println(args[0] + ": admins only")
			default:
				fmt.Println(args[0]+": admins and", strings.Join(access.Users, ", "))
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringSliceVar(&users, "users", nil, "only these users (and admins), comma separated")
	f.BoolVar(&adminsOnly, "admins-only", false, "only admins")
	f.BoolVar(&everyone, "everyone", false, "every signed-in user")
	return cmd
}
