package command

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/shopmonkeyus/go-common/compress"
	"github.com/shopmonkeyus/go-common/logger"
)

func parseLastLines(fn string, n int) (string, error) {
	file, err := os.Open(fn)
	if err != nil {
		return "", err
	}
	defer file.Close()

	stats, statsErr := file.Stat()
	if statsErr != nil {
		return "", statsErr
	}

	buf := make([]byte, stats.Size())
	_, err = file.ReadAt(buf, 0)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(buf), "\n")
	totalLines := len(lines)

	start := totalLines - n
	if start < 0 {
		start = 0
	}

	lastLines := lines[start:]

	return strings.Join(lastLines, "\n"), nil
}

type Uploader func(ctx context.Context, log logger.Logger, file string) (string, error)

type ForkArgs struct {
	// required
	Log     logger.Logger
	Command string

	// optional
	Context             context.Context
	Args                []string
	Cwd                 string
	BaseDir             string // the base director to upload if different than dir
	Dir                 string // the directory to store logs in
	LogFilenameLabel    string
	SaveLogs            bool
	Env                 []string
	SkipBundleOnSuccess bool
}

type ForkResult struct {
	Duration       time.Duration
	LastErrorLines string
	ProcessState   *os.ProcessState
	LogFileBundle  string
}

func (r *ForkResult) String() string {
	pState := ""
	if r.ProcessState != nil {
		pState = r.ProcessState.String()
	}
	return fmt.Sprintf("ProcessState: %s, Duration: %s, LogFileBundle: %s", pState, r.Duration, r.LogFileBundle)
}

var looksLikeJSONRegex = regexp.MustCompile(`^\s*[\[\{]`)

func looksLikeJSON(s string) bool {
	return looksLikeJSONRegex.MatchString(s)
}

func formatCmd(cmdargs []string) string {
	var args []string
	for _, arg := range cmdargs {
		if looksLikeJSON(arg) {
			// quote json so i can paste it out of the logs and into my terminal 😤
			args = append(args, "'"+arg+"'")
		} else {
			args = append(args, arg)
		}
	}
	return fmt.Sprintf("%s %s\n", os.Args[0], strings.Join(args, " "))
}

// Fork will run a command on the current binary
func Fork(args ForkArgs) (*ForkResult, error) {
	started := time.Now()
	if args.Log == nil {
		args.Log = logger.NewConsoleLogger(logger.LevelInfo)
	}
	dir := args.Dir
	executable := os.Args[0]
	if dir == "" {
		tmp, err := os.MkdirTemp("", filepath.Base(executable)+"-")
		if err != nil {
			return nil, fmt.Errorf("error creating temp dir: %w", err)
		}
		defer os.RemoveAll(tmp)
		dir = tmp
	} else {
		if _, err := os.Stat(dir); err != nil && os.IsNotExist(err) {
			os.MkdirAll(dir, 0755)
		}
	}
	cmdargs := append([]string{args.Command}, args.Args...)

	args.Log.Trace("executing: %s", formatCmd(cmdargs))

	ctx := args.Context
	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, executable, cmdargs...)
	if args.Cwd != "" {
		cmd.Dir = args.Cwd
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("error getting current working directory: %w", err)
		}
		cmd.Dir = cwd
	}
	if len(args.Env) > 0 {
		cmd.Env = args.Env
		args.Log.Trace("using custom env")
	} else {
		cmd.Env = os.Environ()
		args.Log.Trace("using default env")
	}

	var err error
	var stderrFn, stdoutFn string
	var stderr, stdout *os.File

	if args.SaveLogs {
		label := args.LogFilenameLabel
		if label == "" {
			label = "job-" + time.Now().Format("20060102-150405")
		}
		stderrFn = filepath.Join(dir, label+"_stderr.txt")
		stdoutFn = filepath.Join(dir, label+"_stdout.txt")

		stderr, err = os.Create(stderrFn)
		if err != nil {
			return nil, fmt.Errorf("error creating temporary stderr log file: %w", err)
		}
		defer stderr.Close()

		stdout, err = os.Create(stdoutFn)
		if err != nil {
			return nil, fmt.Errorf("error creating temporary stdout log file: %w", err)
		}
		defer stdout.Close()
		cmd.Stderr = stderr
		cmd.Stdout = stdout
		stdout.WriteString(fmt.Sprintf("executing: %s\n", formatCmd(cmdargs)))
	} else {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
	}

	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	var result ForkResult
	var resultError error

	if err := cmd.Run(); err != nil {
		if args.SaveLogs {
			stderr.Close()
			stdout.Close()
			lines, _ := parseLastLines(stderrFn, 10)
			if lines == "" {
				lines, _ = parseLastLines(stdoutFn, 10)
			}
			result.LastErrorLines = lines
		}
		resultError = err
	} else if args.SaveLogs {
		stderr.Close()
		stdout.Close()
	}

	result.ProcessState = cmd.ProcessState
	result.Duration = time.Since(started)

	if args.SaveLogs {
		if !args.SkipBundleOnSuccess || resultError != nil {
			baseDir := dir
			if args.BaseDir != "" {
				baseDir = args.BaseDir
			}
			targz, err := tarGzipDir(baseDir)
			if err != nil {
				return nil, fmt.Errorf("error compressing logs: %w", err)
			}
			result.LogFileBundle = targz
		}
	}

	return &result, resultError
}

func tarGzipDir(srcDir string) (string, error) {
	tmpfn, err := os.CreateTemp("", "*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("tmp: %w", err)
	}
	defer tmpfn.Close()

	if err := compress.TarGz(srcDir, tmpfn); err != nil {
		return "", err
	}
	return tmpfn.Name(), nil
}
