// Package steamcmd contains a wrapper for the Steam CLI client (aka. "steamcmd"). The main type to use here is
// SteamCMD, which can be instantiated using the New function. SteamCMD can execute the steamcmd commands in interactive
// and non-interactive modes.
//
// Multiple Command or CommandType can be queued up in a SteamCMD. Commands can be user defined, but it is recommended
// that you use CommandType instead as they will be loaded from a binding mapping. A Command or a CommandType is
// executed immediately only if SteamCMD is in interactive mode.
//
// One final thing to note is that you need the "steamcmd" binary installed on your path for the SteamCMD wrapper to
// work.
package steamcmd

import (
	"bytes"
	"github.com/Netflix/go-expect"
	"github.com/andygello555/agem"
	"github.com/pkg/errors"
	"io"
	"os/exec"
	"strings"
	"time"
)

const (
	// InteractivePrompt is the prompt that SteamCMD uses in interactive mode.
	InteractivePrompt = "Steam>"
	// ExpectTimeout is the timeout for the Expect calls.
	ExpectTimeout = time.Minute
	// WaitTimeout is the amount of time to wait for the process to shut down.
	WaitTimeout = time.Second * 5
)

// SteamCMD is a wrapper for the Steam CLI client (steamcmd). It can run a sequence of Command in both interactive and
// non-interactive modes.
type SteamCMD struct {
	// commands is a list of Command that are queued up.
	commands []*Command
	// stdout is an additional io.Writer to write the stdout of the cmd to. This can be set in NewDebug, but it will be
	// defaulted to io.Discard in the New constructor.
	stdout io.Writer
	// stderr is an additional io.Writer to write the stderr of the cmd to. This can be set in NewDebug, but it will be
	// defaulted to io.Discard in the New constructor.
	stderr io.Writer
	// serialisedCommands is a list of serialised Command (with their args).
	serialisedCommands []string
	// interactive indicates whether the SteamCMD was started in interactive mode.
	interactive bool
	// console is the expect.Console that is used for expecting prompts from SteamCMD when it's running in interactive
	// mode.
	console *expect.Console
	// cmd is the exec.Cmd that is used to manage the SteamCMD process.
	cmd *exec.Cmd
	// before is the buffer of bytes that represent the output of the current Command. This is only used in interactive
	// mode.
	before bytes.Buffer
	// after is the buffer of bytes of the expected prompt in the output of the current Command. This is only used in
	// interactive mode.
	after bytes.Buffer
	// closed is whether SteamCMD.Close has been called before.
	closed bool
	// quitYet is set when the Quit command is first queued/executed.
	quitYet bool
	// ParsedOutputs is the list of parsed outputs from Command.Parse from each queued/executed Command. This means that
	// the output of the third command will lie at index 2.
	ParsedOutputs []any
}

// New creates a new SteamCMD. You can specify whether to run Command in interactive mode or not.
func New(interactive bool) *SteamCMD {
	return NewDebug(interactive, io.Discard, io.Discard)
}

func NewDebug(interactive bool, stdout, stderr io.Writer) *SteamCMD {
	return &SteamCMD{
		commands:           make([]*Command, 0),
		stdout:             stdout,
		stderr:             stderr,
		serialisedCommands: []string{"+login anonymous"},
		interactive:        interactive,
		ParsedOutputs:      make([]any, 0),
	}
}

// setBuffers is called by expectString, and expectEOF to update the after, before, and interactiveBuffer buffers.
func (sc *SteamCMD) setBuffers(serialisedCommand string, read string, expected string) {
	sc.before.Reset()
	sc.before.WriteString(strings.TrimPrefix(read, serialisedCommand))
	sc.before.Truncate(sc.before.Len() - len(expected))
	sc.after.Reset()
	sc.after.WriteString(expected)
}

// expectString will call ExpectString on the console with the given string. It will then set the after buffer to be the
// string read by ExpectString, and the before buffer to be the output that was read from the previous expectString up
// until this one. interactiveBuffer will also be reset to accommodate the next call to expectString.
func (sc *SteamCMD) expectString(serialisedCommand string, s string) error {
	msg, err := sc.console.Expect(expect.String(s), expect.WithTimeout(ExpectTimeout))
	if err != nil {
		return errors.Wrapf(err, "error whilst expecting \"%s\" from interactive SteamCMD", s)
	}
	sc.setBuffers(serialisedCommand, msg, s)
	return nil
}

// closeInteractive will clean up the cmd and console that are used to manage the interactive mode.
func (sc *SteamCMD) closeInteractive() (err error) {
	if sc.cmd != nil {
		// We only add the Quit command if quitYet is not set
		if !sc.quitYet {
			err = sc.AddCommandType(Quit)
		}

		waitErrChan := make(chan error)
		go func() {
			_, waitErr := sc.cmd.Process.Wait()
			waitErrChan <- waitErr
		}()

		var waitErr error
		select {
		case <-time.After(WaitTimeout):
			// If the initial wait times out then we will kill the process. The goroutine that was started above should
			// then wait until the process' resources are cleared.
			err = agem.MergeErrors(err, errors.Wrap(sc.cmd.Process.Kill(), "process kill failed"))
			waitErr = <-waitErrChan
		case waitErr = <-waitErrChan:
			break
		}
		err = agem.MergeErrors(err, errors.Wrap(waitErr, "wait failed"))
		sc.cmd = nil
	}

	if sc.console != nil {
		err = agem.MergeErrors(err, sc.console.Close())
		sc.console = nil
	}

	if err != nil {
		err = errors.Wrap(err, "could not close interactive SteamCMD")
	}
	return
}

// startInteractive mode will set the console and cmd fields that are used to manage the interactive mode.
func (sc *SteamCMD) startInteractive() (err error) {
	if sc.console, err = expect.NewConsole(); err != nil {
		return errors.Wrap(err, "could not start SteamCMD in interactive mode")
	}
	defer func() {
		if err != nil {
			// If an error has occurred whilst starting interactive mode we will close the SteamCMD
			err = agem.MergeErrors(err, sc.closeInteractive())
		}
	}()

	sc.cmd = exec.Command("steamcmd", sc.serialisedCommands...)
	sc.cmd.Stdin = sc.console.Tty()
	sc.cmd.Stdout = io.MultiWriter(sc.console.Tty(), sc.stdout)
	sc.cmd.Stderr = io.MultiWriter(sc.console.Tty(), sc.stderr)
	if err = sc.cmd.Start(); err != nil {
		return errors.Wrap(err, "could not start SteamCMD binary")
	}

	if err = sc.expectString("", InteractivePrompt); err != nil {
		return errors.Wrap(err, "error occurred whilst expecting prompt for SteamCMD")
	}
	return
}

// executeInteractive will execute the given Command immediately when SteamCMD is in interactive mode. The Command will
// be retried until Command.ValidateOutput succeeds.
func (sc *SteamCMD) executeInteractive(command *Command, args ...any) (err error) {
	// Reset the buffers, so we don't get any leaks from the previous command
	sc.before.Reset()
	sc.after.Reset()
	serialisedCommand := command.Serialise(args...)[1:]

	// We keep executing the command until we can validate the output
	tryNo := 0
	for !command.ValidateOutput(tryNo, sc.before.Bytes()) {
		//fmt.Printf("Sending line: \"%s\"\n", serialisedCommand)
		if _, err = sc.console.SendLine(serialisedCommand); err != nil {
			return errors.Wrapf(err, "could not send command \"%s\" to the interactive SteamCMD", serialisedCommand)
		}

		if command.Type != Quit {
			if err = sc.expectString(serialisedCommand, InteractivePrompt); err != nil {
				return errors.Wrapf(err, "could not expect SteamCMD prompt after %s command", command.Type.String())
			}
		}
		tryNo++
		//fmt.Printf("before: \"%s\"\n", sc.before.String())
		//fmt.Printf("after: \"%s\"\n", sc.after.String())
	}

	var parsedOutput any
	if parsedOutput, err = command.Parse(sc.before.Bytes()); err != nil {
		err = errors.Wrapf(err, "could not parse output for command \"%s\"", serialisedCommand)
	}
	sc.ParsedOutputs = append(sc.ParsedOutputs, parsedOutput)
	return
}

// AddCommand will add the given Command to the serialised command string. The Command will not be executed unless
// SteamCMD is running in interactive mode.
func (sc *SteamCMD) AddCommand(command *Command, args ...any) (err error) {
	// If SteamCMD is already closed then return an error
	if sc.closed {
		return errors.New("cannot queue/execute more commands after closing SteamCMD")
	}

	// If we have already quit then we cannot execute any more commands
	if sc.quitYet {
		return errors.New("cannot queue/execute more commands after queuing/executing Quit command")
	}

	if !command.ValidateArgs(args...) {
		err = errors.Errorf("command \"%s\" was given an invalid arg (%v)", command.Type.String(), args)
		return
	}

	// Add the serialised command and the regular command
	//fmt.Printf("Queuing/executing command \"%s\"\n", command.Serialise(args...))
	sc.commands = append(sc.commands, command)
	sc.serialisedCommands = append(sc.serialisedCommands, command.Serialise(args...))

	// Check if the command's type is Quit and set the quitYet flag accordingly
	if command.Type == Quit {
		if sc.quitYet {
			return errors.New("cannot quit SteamCMD more than once")
		}
		sc.quitYet = true
	}

	// If SteamCMD is interactive, then we will execute the command straight away
	if sc.interactive {
		return sc.executeInteractive(command, args...)
	}
	return
}

// AddCommandType will look up the given CommandType in the default command lookup, then add that command using
// AddCommand.
func (sc *SteamCMD) AddCommandType(commandType CommandType, args ...any) (err error) {
	if command, ok := commands[commandType]; ok {
		return sc.AddCommand(&command, args...)
	} else {
		err = errors.Errorf(
			"cannot find command type \"%s\" (%d) in commands lookup",
			commandType.String(), commandType,
		)
	}
	return
}

// Start will start the SteamCMD process, if it is in interactive mode. Otherwise, nothing will happen.
func (sc *SteamCMD) Start() (err error) {
	if sc.interactive {
		if sc.closed {
			return errors.New("cannot start a SteamCMD that is closed")
		}
		return sc.startInteractive()
	}
	return
}

// Close will stop the SteamCMD process, if it is in interactive mode. Otherwise, the command will be executed all at
// once.
func (sc *SteamCMD) Close() (err error) {
	if !sc.closed {
		// Only set closed when we have closed the SteamCMD without errors
		defer func() {
			if err == nil {
				sc.closed = true
			}
		}()

		// If SteamCMD is interactive, we delegate closing to closeInteractive
		if sc.interactive {
			return sc.closeInteractive()
		}

		// We add a quit command if the user hasn't yet
		if !sc.quitYet {
			if err = sc.AddCommandType(Quit); err != nil {
				return errors.Wrap(err, "could not add Quit command to a non-interactive SteamCMD execution")
			}
		}

		// Execute the non-interactive command all at once
		var stdout bytes.Buffer
		sc.cmd = exec.Command("steamcmd", sc.serialisedCommands...)
		sc.cmd.Stdout = &stdout
		if err = sc.cmd.Run(); err != nil {
			return errors.Wrapf(err, "could not run non-interactive series of commands for SteamCMD (%v)", sc.serialisedCommands)
		}

		// Parse the output for each command
		for i, command := range sc.commands {
			var parsedOutput any
			if parsedOutput, err = command.Parse(stdout.Bytes()); err != nil {
				return errors.Wrapf(err, "could not parse output for command \"%s\"", sc.serialisedCommands[i])
			}
			sc.ParsedOutputs = append(sc.ParsedOutputs, parsedOutput)
		}
		return
	} else {
		return errors.New("cannot close a SteamCMD that has already been closed")
	}
}

// CommandWithArgs simply serves as a wrapper for the arguments that are passed to SteamCMD.Flow.
type CommandWithArgs struct {
	Command *Command
	Args    []any
}

// NewCommandWithArgs creates a new CommandWithArgs using the given CommandType and args. This is so you can just use
// the output of this function directly in the input to SteamCMD.Flow.
func NewCommandWithArgs(commandType CommandType, args ...any) *CommandWithArgs {
	var (
		command Command
		ok      bool
	)

	if command, ok = commands[commandType]; !ok {
		command = commands[Quit]
	}
	return &CommandWithArgs{
		Command: &command,
		Args:    args,
	}
}

// Flow will start the SteamCMD by running SteamCMD.Start, queue up a flow of CommandWithArgs one at a time, then finally
// call Close on the SteamCMD.
func (sc *SteamCMD) Flow(commandWithArgs ...*CommandWithArgs) (err error) {
	defer func(sc *SteamCMD) {
		err = agem.MergeErrors(err, errors.Wrap(sc.Close(), "cannot close flow"))
	}(sc)

	if err = sc.Start(); err != nil {
		return errors.Wrap(err, "could not start flow")
	}

	for i, command := range commandWithArgs {
		//fmt.Printf("CommandWithArgs no. %d: \"%s\"\n", i, command.Command.Serialise(command.Args...))
		if err = sc.AddCommand(command.Command, command.Args...); err != nil {
			return errors.Wrapf(
				err, "could not queue/execute command no. %d (%s)",
				i, command.Command.Serialise(command.Args...),
			)
		}
	}
	return
}
