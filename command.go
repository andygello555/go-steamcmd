package steamcmd

import (
	"bytes"
	"fmt"
	"github.com/hjson/hjson-go/v4"
	"github.com/pkg/errors"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// ArgType is the type of an Arg. It represents how an Arg should be serialised and parsed.
type ArgType int

const (
	// Number represents values of type: int, int8, int16, int32, int64, float32, float64.
	Number ArgType = iota
	// String represents string values.
	String
)

// String returns the string representation of the ArgType.
func (at ArgType) String() string {
	switch at {
	case Number:
		return "Number"
	case String:
		return "String"
	default:
		return "<nil>"
	}
}

// ParseArgType first checks if the given string can be parsed to an int, then whether it can be parsed to a float.
// If it can be parsed to either of these then Number is returned. Otherwise, String is returned. It also returns the
// value that the arg should have.
func ParseArgType(s string) (any, ArgType) {
	var (
		intVal, floatVal any
		isInt, isFloat   error
	)
	intVal, isInt = strconv.ParseInt(s, 10, 64)
	floatVal, isFloat = strconv.ParseFloat(s, 64)

	switch {
	case isInt == nil:
		return intVal, Number
	case isFloat == nil:
		return floatVal, Number
	default:
		return s, String
	}
}

// DefaultSerialiser serialises the given value to a string using the default logic for the ArgType.
func (at ArgType) DefaultSerialiser(value any) string {
	switch at {
	case Number:
		switch value.(type) {
		case int, int8, int16, int32, int64:
			v := reflect.ValueOf(value)
			return strconv.Itoa(int(v.Int()))
		case uint, uint8, uint16, uint32, uint64:
			v := reflect.ValueOf(value)
			return strconv.Itoa(int(v.Uint()))
		case float32, float64:
			v := reflect.ValueOf(value)
			return fmt.Sprintf("%f", v.Float())
		default:
			panic(errors.Errorf(
				"cannot serialise a %s that has the value %v (type: %s)",
				at.String(), value, reflect.TypeOf(value).String()),
			)
		}
	case String:
		return value.(string)
	default:
		return "<nil>"
	}
}

// DefaultValidator checks if the given value fits the ArgType.
func (at ArgType) DefaultValidator(value any) bool {
	switch at {
	case Number:
		switch value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
			return true
		default:
			return false
		}
	case String:
		_, ok := value.(string)
		return ok
	default:
		return false
	}
}

type ArgValidator func(any) bool
type ArgSerialiser func(any) string

type Arg struct {
	Name       string
	Type       ArgType
	Required   bool
	Validator  ArgValidator
	Serialiser ArgSerialiser
}

// Serialise the given value to a string using the Serialiser for the Arg. If there is no Serialiser for the Arg then
// the ArgType.DefaultSerialiser will be used instead.
func (a *Arg) Serialise(value any) string {
	if a.Serialiser != nil {
		return a.Serialiser(value)
	}
	return a.Type.DefaultSerialiser(value)
}

// Validate the given value against the Type of the Arg and the Validator for the Arg (if there is one).
func (a *Arg) Validate(value any) bool {
	if a.Type.DefaultValidator(value) {
		if a.Validator != nil {
			return a.Validator(value)
		}
		return true
	}
	return false
}

// CommandType represents a (sub)command that can be executed by SteamCMD.
type CommandType int

const (
	// AppInfoPrint calls the "app_info_print" command. It takes a sole Number as an Arg.
	AppInfoPrint CommandType = iota
	// Quit calls the "quit" command. It takes no arguments.
	Quit
)

// String returns the SteamCMD representation of the CommandType that will be used to call the command in the
// steamcmd binary.
func (ct CommandType) String() string {
	switch ct {
	case AppInfoPrint:
		return "app_info_print"
	case Quit:
		return "quit"
	default:
		return "<nil>"
	}
}

// CommandTypeFromString looks up the given string as a CommandType.
func CommandTypeFromString(s string) (CommandType, error) {
	switch s {
	case "AppInfoPrint":
		return AppInfoPrint, nil
	case "Quit":
		return Quit, nil
	default:
		return CommandType(0), fmt.Errorf("cannot get CommandType from \"%s\"", s)
	}
}

// CommandOutputValidator validates whether a Command has completed successfully by validating the output of the
// Command as well as which try the command is currently on.
type CommandOutputValidator func(tryNo int, output []byte) bool

// CommandOutputParser parses the output of a Command to a more usable format. Usually, JSON (map[string]any).
type CommandOutputParser func(output []byte) (any, error)

// Command represents a command that can be executed in SteamCMD. User defined Command are possible, but users should
// stick to executing Commands via their CommandType instead.
type Command struct {
	Type      CommandType
	Parser    CommandOutputParser
	Validator CommandOutputValidator
	Args      []*Arg
}

// Serialise will return the string that will be used to execute this Command via the steamcmd binary.
func (c *Command) Serialise(args ...any) string {
	command := []string{fmt.Sprintf("+%s", c.Type.String())}
	if len(args) > 0 && len(c.Args) > 0 {
		for i, arg := range c.Args {
			if i < len(args) {
				command = append(command, arg.Serialise(args[i]))
			}
		}
	}
	return strings.Join(command, " ")
}

// ValidateArgs will validate the given args against the Arg.Validator for each Arg in Args. If the number of args given
// exceeds the number of Arg in Args, then this will count as invalid. If a required Arg is not provided, this will also
// count as invalid.
func (c *Command) ValidateArgs(args ...any) bool {
	if len(args) > len(c.Args) {
		return false
	}

	valid := true
	if len(args) > 0 && len(c.Args) > 0 {
		for i, arg := range c.Args {
			if i < len(args) {
				value := args[i]
				if !arg.Validate(value) {
					valid = false
					break
				}
			} else {
				if arg.Required {
					valid = false
				}
				break
			}
		}
	}
	return valid
}

// Parse the Command's output using their Parser, if it is not nil. Otherwise, the output will just be converted to a
// string and returned.
func (c *Command) Parse(out []byte) (any, error) {
	if c.Parser != nil {
		return c.Parser(out)
	}
	return string(out), nil
}

// ValidateOutput of the Command by using the Validator of the Command. It also must be given the current try for the
// Command. When SteamCMD is in interactive mode we might keep executing a Command until we can validate its output.
//
// If the Command.Validator is nil, then we will return tryNo > 0. This is useful for the Quit command that should be
// executed at least once but has no output to validate.
func (c *Command) ValidateOutput(tryNo int, out []byte) bool {
	if c.Validator == nil {
		return tryNo > 0
	}
	return c.Validator(tryNo, out)
}

// commands contains the default Command bindings for SteamCMD.
var commands = map[CommandType]Command{
	AppInfoPrint: {
		Type: AppInfoPrint,
		Parser: func(b []byte) (any, error) {
			// SteamCMD object syntax (notice lack of ":"):
			// "hello"
			// {
			//    "name"   "bob"
			// }
			b = bytes.Trim(b, " \t\r\n\x1b[1m\n")
			indices := regexp.MustCompile(`"\d+"`).FindStringIndex(string(b))
			// Remove the header of the response
			jsonBody := strings.TrimSpace(string(b)[indices[1]+1:])
			//fmt.Println("jsonBody 1", strings.Join(strings.Split(jsonBody, "\r\n")[:200], "\r\n"))
			//fmt.Printf("jsonBody 1\n%q\n", jsonBody)
			// Replace openings of json Objects with the correct syntax.
			jsonBody = regexp.MustCompile(`"([^"]+)"\r{0,2}\n\t+\{`).ReplaceAllString(jsonBody, "\"$1\": {")
			//fmt.Println("jsonBody 2", strings.Join(strings.Split(jsonBody, "\r\n")[:200], "\r\n"))
			//fmt.Printf("jsonBody 2\n%q\n", jsonBody)
			// Replace key-value pairs with proper JSON syntax
			jsonBody = regexp.MustCompile(`"([^"]+)"\t\t"(([^\\]\\"|[^"])*?)"`).ReplaceAllString(jsonBody, "\"$1\": '''$2\n'''")
			//fmt.Println("jsonBody 3", strings.Join(strings.Split(jsonBody, "\r\n")[:200], "\r\n"))
			//fmt.Printf("jsonBody 3\n%q\n", jsonBody)

			var json map[string]any
			if err := hjson.Unmarshal([]byte(jsonBody), &json); err != nil {
				return jsonBody, err
			}
			return json, nil
		},
		Validator: func(tryNo int, b []byte) bool {
			return regexp.MustCompile(`, change number : [1-9]`).Match(b)
		},
		Args: []*Arg{
			{
				Name:     "appid",
				Type:     Number,
				Required: true,
			},
		},
	},
	Quit: {Type: Quit},
}
