package cmd

import (
	"fmt"
	"github.com/jenkins-x/jx/pkg/jx/cmd/helper"
	"strings"

	v1 "github.com/jenkins-x/jx/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx/pkg/client/clientset/versioned"

	"github.com/jenkins-x/jx/pkg/users"

	"github.com/jenkins-x/jx/pkg/jx/cmd/opts"
	"github.com/jenkins-x/jx/pkg/jx/cmd/templates"
	"github.com/jenkins-x/jx/pkg/kube"
	"github.com/jenkins-x/jx/pkg/log"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/spf13/cobra"
	"gopkg.in/AlecAivazis/survey.v1"
)

// DeleteUserOptions are the flags for delete commands
type DeleteUserOptions struct {
	*opts.CommonOptions

	SelectAll    bool
	SelectFilter string
	Confirm      bool
}

var (
	deleteUserLong = templates.LongDesc(`
		Deletes one or more users 
`)

	deleteUserExample = templates.Examples(`
		# Delete the user with the login of cheese
		jx delete user cheese 
	`)
)

// NewCmdDeleteUser creates a command object
// retrieves one or more resources from a server.
func NewCmdDeleteUser(commonOpts *opts.CommonOptions) *cobra.Command {
	options := &DeleteUserOptions{
		CommonOptions: commonOpts,
	}

	cmd := &cobra.Command{
		Use:     "user",
		Short:   "Deletes one or more users",
		Long:    deleteUserLong,
		Example: deleteUserExample,
		Aliases: []string{"users"},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			helper.CheckErr(err)
		},
	}

	cmd.Flags().BoolVarP(&options.SelectAll, "all", "a", false, "Should we default to selecting all the matched users for deletion")
	cmd.Flags().StringVarP(&options.SelectFilter, "filter", "f", "", "Fitlers the list of users you can pick from")
	cmd.Flags().BoolVarP(&options.Confirm, "yes", "y", false, "Confirms we should uninstall this installation")
	return cmd
}

// Run implements this command
func (o *DeleteUserOptions) Run() error {
	surveyOpts := survey.WithStdio(o.In, o.Out, o.Err)
	err := o.RegisterUserCRD()
	if err != nil {
		return err
	}

	jxClient, ns, err := o.JXClientAndAdminNamespace()
	if err != nil {
		return err
	}

	_, userNames, err := users.GetUsers(jxClient, ns)
	if err != nil {
		return err
	}

	names := o.Args
	if len(names) == 0 {
		if o.BatchMode {
			return fmt.Errorf("Missing user login name argument")
		}
		names, err = util.SelectNamesWithFilter(userNames, "Which users do you want to delete: ", o.SelectAll, o.SelectFilter, "", o.In, o.Out, o.Err)
		if err != nil {
			return err
		}
	}

	if o.BatchMode {
		if !o.Confirm {
			return fmt.Errorf("In batch mode you must specify the '-y' flag to confirm")
		}
	} else {
		log.Warnf("You are about to delete these users '%s'. This operation CANNOT be undone!",
			strings.Join(names, ","))

		flag := false
		prompt := &survey.Confirm{
			Message: "Are you sure you want to delete these all these users?",
			Default: false,
		}
		err = survey.AskOne(prompt, &flag, nil, surveyOpts)
		if err != nil {
			return err
		}
		if !flag {
			return nil
		}
	}

	for _, name := range names {
		err = o.deleteUser(name)
		if err != nil {
			log.Warnf("Failed to delete user %s: %s\n", name, err)
		} else {
			log.Infof("Deleted user %s\n", util.ColorInfo(name))
		}
		log.Infof("Attempting to unbind user %s from associated role\n", util.ColorInfo(name))
		err = o.deleteUserFromRoleBindings(name, ns, jxClient)
		if err != nil {
			log.Warnf("Problem to unbind user %s from associated role\n", util.ColorWarning(name))
		}
	}
	return nil
}

func (o *DeleteUserOptions) deleteUser(name string) error {
	jxClient, devNs, err := o.JXClientAndDevNamespace()
	if err != nil {
		return err
	}
	kubeClient, err := o.KubeClient()
	if err != nil {
		return err
	}
	ns, err := kube.GetAdminNamespace(kubeClient, devNs)
	if err != nil {
		return err
	}
	return users.DeleteUser(jxClient, ns, name)
}

func (o *DeleteUserOptions) deleteUserFromRoleBindings(name string, ns string, jxClient versioned.Interface) error {
	kubeClient, err := o.KubeClient()
	if err != nil {
		return err
	}

	roles, roleNames, err := kube.GetTeamRoles(kubeClient, ns)
	if err != nil {
		return err
	}
	if len(roleNames) == 0 {
		return nil
	}

	return kube.UpdateUserRoles(kubeClient, jxClient, ns, v1.UserTypeLocal, name, nil, roles)
}
