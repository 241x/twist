// ExitError 携带退出码的错误类型。退出码 1 为用户侧错误，2 为运行时错误。
package cmd

import "errors"

// ExitError 可携带退出码的错误类型。
type ExitError struct {
	Code int
	Msg  string
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Msg + ": " + e.Err.Error()
	}
	return e.Msg
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

func GetExitCode(err error) int {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 1
}
