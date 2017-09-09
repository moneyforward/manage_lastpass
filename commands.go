package main

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"io/ioutil"
	lc "lastpass_provisioning/lastpass_client"
	lf "lastpass_provisioning/lastpass_format"
	"lastpass_provisioning/logger"
	"lastpass_provisioning/service"
	"lastpass_provisioning/util"
	"os"
	"strings"
	"sync"
	"time"
)

func init() {
	// Requirements:
	// - .Description: First and last line is blank.
	// - .ArgsUsage: ArgsUsage includes flag usages (e.g. [-v|verbose] <hostId>).
	//   All cli.Command should have ArgsUsage field.
	cli.CommandHelpTemplate = `NAME:
   {{.HelpName}} - {{.Usage}}
USAGE:
   {{.HelpName}} {{if .ArgsUsage}}{{.ArgsUsage}}{{else}}[arguments...]{{end}}{{if .Description}}
DESCRIPTION:{{.Description}}{{end}}{{if .VisibleFlags}}
OPTIONS:
   {{range .VisibleFlags}}{{.}}
   {{end}}{{end}}
`
}

// NewLastPassClientFromContext creates LastpassClient.
// This method depends on urfave/cli.
func NewLastPassClientFromContext(c *cli.Context) *lc.LastPassClient {
	confFile := c.GlobalString("config")
	return lc.NewLastPassClient(confFile)
}

// Commands cli.Command object list
var Commands = []cli.Command{
	commandCreate,
	commandGet,
	commandDescribe,
	commandDelete,
	commandUpdate,
	subCommandDisableMFA,
	subCommandResetPassword,
}

var location = time.UTC

// Update command with subcommands
var commandUpdate = cli.Command{
	Name:  "update",
	Usage: "update specific object",
	Subcommands: []cli.Command{
		subCommandUpdateUser,
	},
}

var subCommandDisableMFA = cli.Command{
	Name:      "disable-mfa",
	Usage:     "disable mfa of user <email>",
	ArgsUsage: "<email>",
	Action:    doDisableMFA,
}

func doDisableMFA(c *cli.Context) error {
	argUserName := c.Args().Get(0)
	if argUserName == "" {
		logger.DieIf(errors.New("Email(username) has to be specified"))
	}

	client := NewLastPassClientFromContext(c)
	s := service.NewUserService(client)

	status, err := s.DisableMultifactor(argUserName)
	logger.DieIf(err)
	logger.Log(c.Command.Name, status.String())
	return nil
}

var subCommandResetPassword = cli.Command{
	Name:      "reset-password",
	Usage:     "reset password of user <email>",
	ArgsUsage: "<email>",
	Action:    doResetPassword,
}

func doResetPassword(c *cli.Context) error {
	argUserName := c.Args().Get(0)
	if argUserName == "" {
		logger.DieIf(errors.New("Email(username) has to be specified"))
	}

	client := NewLastPassClientFromContext(c)
	s := service.NewUserService(client)

	status, err := s.ResetPassword(argUserName)
	logger.DieIf(err)
	logger.Log(c.Command.Name, status.String())
	return nil
}

var subCommandUpdateUser = cli.Command{
	Name:        "user",
	Usage:       "update user <email>",
	Description: `update a <email>`,
	ArgsUsage:   "<email>",
	Subcommands: []cli.Command{
		{
			Name:        "department",
			Usage:       "update user department",
			ArgsUsage:   "[[--leave | -l <department>]...] [[--join | -j <department>]...]",
			Description: "User can specify either --leave or --join to move department",
			Action:      doUpdateBelongingDepartment,
			Flags: []cli.Flag{
				cli.StringSliceFlag{Name: "leave, l", Value: &cli.StringSlice{}, Usage: "leave current department"},
				cli.StringSliceFlag{Name: "join, j", Value: &cli.StringSlice{}, Usage: "join new department"},
			},
		},
	},
}

func doUpdateBelongingDepartment(c *cli.Context) error {
	argUserName := c.Args().Get(0)
	if argUserName == "" {
		logger.DieIf(errors.New("Email(username) has to be specified"))
	}

	client := NewLastPassClientFromContext(c)
	s := service.NewUserService(client)

	// Fetch User if he/she exists
	user, err := s.GetUserData(argUserName)
	logger.DieIf(err)

	// Join
	user.Groups = append(user.Groups, c.StringSlice("join")...)

	// Leave
	leave := c.StringSlice("leave")
	for i := 0; i < len(leave); i++ {
		newDeps := []string{}
		for _, dep := range user.Groups {
			if dep != leave[i] {
				newDeps = append(newDeps, dep)
			}
		}
		user.Groups = newDeps
	}

	// Update
	err = s.UpdateUser(user)
	logger.DieIf(err)
	logger.Log("updated", user.UserName)
	return nil
}

// Delete command with subcommands
var commandDelete = cli.Command{
	Name:  "delete",
	Usage: "delete specific object",
	Subcommands: []cli.Command{
		subCommandDeleteUser,
	},
}

var subCommandDeleteUser = cli.Command{
	Name:        "user",
	Usage:       "delete user <email>",
	Description: `delete a <email> by choosing either 'deactivate(default)' or 'delete'`,
	ArgsUsage:   "[--mode | -m <deleteMode>] <email>",
	Action:      doDeleteUser,
	Flags: []cli.Flag{
		cli.StringFlag{Name: "mode, m", Value: "deactivate", Usage: "deleteMode"},
	},
}

func doDeleteUser(c *cli.Context) error {
	argUserName := c.Args().Get(0)
	if argUserName == "" {
		logger.DieIf(errors.New("Email(username) has to be specified"))
	}

	var mode = service.DeactivationMode(service.Deactivate)
	switch c.String("mode") {
	case "deactivate":
		mode = service.Deactivate
	case "delete":
		mode = service.Delete
	default:
		mode = service.Deactivate
	}

	client := NewLastPassClientFromContext(c)
	err := service.NewUserService(client).DeleteUser(argUserName, mode)
	logger.DieIf(err)
	logger.Log(c.String("mode"), argUserName)
	return nil
}

// Describe command with subcommands
var commandDescribe = cli.Command{
	Name:  "describe",
	Usage: "describe specific object",
	Subcommands: []cli.Command{
		subCommandDescribeUser,
	},
}

var subCommandDescribeUser = cli.Command{
	Name:        "user",
	Usage:       "describe user",
	Description: `Show the information of the user with <email>`,
	ArgsUsage:   "<email>",
	Action:      doDescribeUser,
}

func doDescribeUser(c *cli.Context) error {
	argUserName := c.Args().Get(0)
	if argUserName == "" {
		logger.DieIf(errors.New("Email(username) has to be specified"))
	}

	client := NewLastPassClientFromContext(c)
	user, err := service.NewUserService(client).GetUserData(argUserName)
	logger.DieIf(err)

	util.PrintIndentedJSON(user)
	return nil
}

// Get command with subcommands
var commandGet = cli.Command{
	Name:  "get",
	Usage: "Get objects",
	Subcommands: []cli.Command{
		subCommandDashboards,
		subCommandGetUsers,
		subCommandGetGroups,
		subCommandGetEvents,
	},
}

func updateLocation(context *cli.Context) (err error) {
	var newLoc *time.Location
	if context.GlobalString("timezone") != "" {
		newLoc, err = time.LoadLocation(context.GlobalString("timezone"))
		if err != nil {
			return err
		}
	}
	location = newLoc
	return nil
}

var subCommandGetEvents = cli.Command{
	Name:        "events",
	Usage:       "get events",
	Description: "Get LastPass events. By default, it retrieves events of all users within that day.",
	ArgsUsage:   "[--user, -u <email> | --duration, -d <days> | [--verbose | -v]]",
	Before: updateLocation,
	Action:      doGetEvents,
	Flags: []cli.Flag{
		cli.IntFlag{Name: "duration, d", Value: 1, Usage: "By specifying this, events from d-day ago to today is retrieved."},
		cli.StringFlag{Name: "user, u", Value: "", Usage: "Specify events for interested users."},
		cli.BoolFlag{Name: "verbose, v", Usage: "Verbose output mode"},
	},
}

func doGetEvents(c *cli.Context) error {
	if c.Bool("verbose") {
		os.Setenv("DEBUG", "1")
	}

	lastPassLoc, _ := time.LoadLocation(lf.LastPassTimeZone)
	now := time.Now().In(lastPassLoc)
	dayAgo := now.Add(-time.Duration(c.Int("duration")) * time.Hour * 24)
	from := lf.JsonLastPassTime{JsonTime: dayAgo}
	to := lf.JsonLastPassTime{JsonTime: now}

	var events *service.Events
	var err error
	s := service.NewEventService(NewLastPassClientFromContext(c))
	switch user := c.String("user"); strings.ToLower(user) {
	case "":
		events, err = s.GetEventReport(user, "", from, to)
	case "api":
		events, err = s.GetAPIEventReports(from, to)
	default:
		events, err = s.GetAllEventReports(from, to)
	}
	if c.String("user") == "" {
		events, err = s.GetAllEventReports(from, to)
	} else {

	}
	logger.DieIf(err)

	events.ConvertTimezone(location)
	util.PrintIndentedJSON(events)
	return err
}

var subCommandGetGroups = cli.Command{
	Name:   "groups",
	Usage:  "get groups",
	Action: doGetGroups,
}

// There are no API that fetches group info
func doGetGroups(c *cli.Context) error {
	client := NewLastPassClientFromContext(c)
	s := service.NewUserService(client)
	users, err := s.GetAllUsers()
	logger.DieIf(err)

	deps := make(map[string]bool)
	for _, u := range users {
		for _, group := range u.Groups {
			if _, ok := deps[group]; !ok {
				deps[group] = true
			}
		}
	}

	for dep := range deps {
		fmt.Println(dep)
	}
	return nil
}

var subCommandGetUsers = cli.Command{
	Name:        "users",
	Usage:       "get users",
	ArgsUsage:   "[--filter, -f <option>]",
	Description: "Use --filter to filter users. You can choose from either `non2fa`, `inactive`, `disabled`, or `admin`",
	Action:      doGetUsers,
	Flags: []cli.Flag{
		cli.StringFlag{Name: "filter, f", Value: "all", Usage: "Filter fetching users"},
	},
}

func doGetUsers(c *cli.Context) (err error) {
	client := NewLastPassClientFromContext(c)
	s := service.NewUserService(client)

	var users []service.User

	switch c.String("filter") {
	case "non2fa":
		users, err = s.GetNon2faUsers()
	case "inactive":
		users, err = s.GetInactiveUsers()
	case "disabled":
		users, err = s.GetDisabledUsers()
	case "admin":
		users, err = s.GetAdminUserData()
	default:
		users, err = s.GetAllUsers()
	}
	logger.DieIf(err)
	service.PrintUserNames(users)
	return nil
}

var commandCreate = cli.Command{
	Name:  "create",
	Usage: "Create a new object",
	Subcommands: []cli.Command{
		subCommandCreateUser,
	},
}

var subCommandCreateUser = cli.Command{
	Name:        "user",
	Usage:       "create an users",
	ArgsUsage:   "[--bulk | -b <file>] [--dept | -d <department>] <username>",
	Description: `Create one or more users specifying either username or pre-configured file.`,
	Action:      doAddUser,
	Flags: []cli.Flag{
		cli.StringFlag{Name: "email, e", Value: "", Usage: "Create user with <email>"},
		cli.StringSliceFlag{Name: "dept, d", Value: &cli.StringSlice{}, Usage: "Create user with <email> in <department>"},
		cli.StringFlag{Name: "bulk, b", Value: "", Usage: "Load users from a JSON <file>"},
	},
}

func doAddUser(c *cli.Context) error {
	if c.String("bulk") != "" {
		return doAddUsersInBulk(c)
	}

	argUserName := c.Args().Get(0)
	if argUserName == "" {
		logger.DieIf(errors.New("Email(username) has to be specified"))
	}

	user := service.User{
		UserName: argUserName,
		Groups:   c.StringSlice("dept"),
	}

	client := NewLastPassClientFromContext(c)
	err := service.NewUserService(client).BatchAdd([]service.User{user})
	logger.DieIf(err)

	message := user.UserName
	for _, dep := range user.Groups {
		message += fmt.Sprintf(" in %v", dep)
	}
	logger.Log("created", message)
	return nil
}

func doAddUsersInBulk(c *cli.Context) error {
	users, err := loadAddingUsers(c.String("bulk"))
	if err != nil {
		logger.DieIf(err)
	}

	client := NewLastPassClientFromContext(c)
	err = service.NewUserService(client).BatchAdd(users)
	logger.DieIf(err)

	for _, user := range users {
		message := user.UserName
		for _, dep := range user.Groups {
			message += fmt.Sprintf(" in %v", dep)
		}
		logger.Log("created", message)
	}

	return err
}

func loadAddingUsers(usersFile string) (config []service.User, err error) {
	f, err := ioutil.ReadFile(usersFile)
	if err != nil {
		return
	}

	data := struct {
		Data []service.User `json:"data"`
	}{}

	if err = json.Unmarshal(f, &data); err != nil {
		return
	}
	config = data.Data
	return
}

var subCommandDashboards = cli.Command{
	Name:        "dashboard",
	Usage:       "Report summary",
	ArgsUsage:   "[--verbose | -v] [--period | -d <duration>]",
	Description: `show audit related dashboard`,
	Action:      doDashboard,
	Before: updateLocation,
	Flags: []cli.Flag{
		cli.IntFlag{Name: "duration, d", Usage: "Audits for past <duration> day"},
		cli.BoolFlag{Name: "verbose, v", Usage: "Verbose output mode"},
	},
}

func doDashboard(c *cli.Context) error {
	start := time.Now()
	if c.Bool("verbose") {
		os.Setenv("DEBUG", "1")
	}

	durationToAuditInDay := 1
	if c.Int("duration") >= 1 {
		durationToAuditInDay = c.Int("duration")
	}

	client := NewLastPassClientFromContext(c)

	folders := []service.SharedFolder{}
	events := []service.Event{}
	organizationMap := make(map[string][]service.User)

	c1 := make(chan []service.User)
	c2 := make(chan []service.SharedFolder)
	c3 := make(chan *service.Events)

	// Fetch Data
	numOfGoRoutines := 3	// Change this number based on goroutine to fetch data from LastPass.
	var wg sync.WaitGroup
	wg.Add(numOfGoRoutines)
	go getAllUsers(&wg, service.NewUserService(client), c1)
	go getSharedFolders(&wg, service.NewFolderService(client), c2)
	go getEvents(&wg, service.NewEventService(client), c3, time.Duration(durationToAuditInDay))
	for i := 0; i < numOfGoRoutines; i++ {
		select {
		case users := <-c1:
			for _, u := range users {
				if u.IsAdmin {
					organizationMap["admin"] = append(organizationMap["admin"], u)
					continue
				}
				for _, group := range u.Groups {
					organizationMap[group] = append(organizationMap[group], u)
				}
			}
		case folders = <-c2:
		case es := <-c3:
			events = es.Events
		}
	}
	wg.Wait()

	// Pull Admin Users from fetched data. Output string is also constructed
	out := fmt.Sprintf("# Admin Users\n")
	for _, u := range organizationMap["admin"] {
		out = out + fmt.Sprintf("- %v\n", u.UserName)
		for _, event := range events {
			if u.UserName == event.Username {
				out = out + fmt.Sprintf("	- %v\n", event.String(location))
			}
		}
	}

	// Pull Activities done through LastPassAPI
	out = out + fmt.Sprintf("# API Activities\n")
	for _, event := range events {
		if event.Username == "API" {
			out = out + fmt.Sprintf("%v\n", event.String(location))
		}
	}

	// Pull activities to be audited such as re-uses of LastPassword master-password.
	out = out + fmt.Sprintf("\n# Audit Events\n")
	for _, event := range events {
		if event.IsAuditEvent() {
			out = out + fmt.Sprintf("%v\n", event.String(location))
		}
	}

	// Check anyone who can access super-admin credentials on critical infrastructure.
	out = out + fmt.Sprintf("\n# Super-Shared Folders\n")
	for _, folder := range folders {
		if folder.ShareFolderName == "Super-Admins" {
			for _, u := range folder.Users {
				out = out + fmt.Sprintf("- "+u.UserName+"\n")
			}
		}
	}

	// Check disabled users. They may be required to be deleted.
	out = out + fmt.Sprintf("\n# Disabled Users\n")
	for _, us := range organizationMap {
		for _, u := range us {
			if u.Disabled {
				out = out + fmt.Sprintf("- "+u.UserName+"\n")
			}
		}
	}

	// Check inactive users who never logged in.
	out = out + fmt.Sprintf("\n# Inactive Users")
	for dep, us := range organizationMap {
		count := 0
		for _, u := range us {
			if u.NeverLoggedIn {
				if count == 0 {
					out = out + fmt.Sprintf("\n## %v", dep)
				}
				out = out + fmt.Sprintf("\n- "+u.UserName)
				count += 1
			}
		}
	}

	// Check users who haven't set 2FA.
	out = out + fmt.Sprintf("\n\n# Non2FA Users")
	for dep, us := range organizationMap {
		count := 0
		for _, u := range us {
			if !u.Is2FA() {
				if count == 0 {
					out = out + fmt.Sprintf("\n## %v", dep)
				}
				out = out + fmt.Sprintf("\n- "+u.UserName)
				count += 1
			}
		}
	}

	fmt.Println(out)

	fmt.Println(time.Since(start))
	return nil
}

func getAllUsers(wg *sync.WaitGroup, s *service.UserService, q chan []service.User) {
	defer wg.Done()
	users, err := s.GetAllUsers()
	logger.DieIf(err)
	q <- users
}

func getEvents(wg *sync.WaitGroup, s *service.EventService, q chan *service.Events, d time.Duration) {
	defer wg.Done()
	loc, _ := time.LoadLocation(lf.LastPassTimeZone)
	now := time.Now().In(loc)
	dayAgo := now.Add(-d * time.Hour * 24)

	from := lf.JsonLastPassTime{JsonTime: dayAgo}
	to := lf.JsonLastPassTime{JsonTime: now}
	events, err := s.GetAllEventReports(from, to)
	logger.DieIf(err)
	q <- events
}

func getSharedFolders(wg *sync.WaitGroup, s *service.FolderService, q chan []service.SharedFolder) {
	defer wg.Done()
	folders, err := s.GetSharedFolders()
	logger.DieIf(err)
	q <- folders
}