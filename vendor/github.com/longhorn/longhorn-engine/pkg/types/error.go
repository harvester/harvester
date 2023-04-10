package types

import (
	"encoding/json"
	"fmt"

	"google.golang.org/grpc/status"
)

type ErrorCode string

const (
	ErrorCodeResultUnknown                   = ErrorCode("ResultUnknown")
	ErrorCodeFunctionFailedWithoutRollback   = ErrorCode("FunctionFailedWithoutRollback")
	ErrorCodeFunctionFailedRollbackSucceeded = ErrorCode("FunctionFailedRollbackSucceeded")
	ErrorCodeFunctionFailedRollbackFailed    = ErrorCode("FunctionFailedRollbackFailed")
)

const (
	CannotRequestHashingSnapshotPrefix = "cannot request hashing snapshot"
)

type Error struct {
	Code            ErrorCode `json:"code"`
	Message         string    `json:"message"`
	RollbackMessage string    `json:"rollbackMessage"`
}

func NewError(code ErrorCode, msg, rollbackMsg string) *Error {
	return &Error{
		Code:            code,
		Message:         msg,
		RollbackMessage: rollbackMsg,
	}
}

func (e *Error) Error() string {
	if e.RollbackMessage == "" {
		return fmt.Sprintf("error: code = %v, message = %s", e.Code, e.Message)
	}
	return fmt.Sprintf("error: code = %v, message = %s, rollbackMessage: %s", e.Code, e.Message, e.RollbackMessage)
}

func (e *Error) ToJSONString() string {
	s, err := json.Marshal(e)
	if err != nil {
		return fmt.Sprintf(`{"code":"ResultUnknown","message":"Cannot marshal to json string for error: %s","rollbackMessage":""}`, e.Error())
	}
	return string(s)
}

func WrapError(in error, format string, args ...interface{}) error {
	if in == nil {
		return nil
	}

	out, ok := in.(*Error)
	if !ok {
		out = NewError(ErrorCodeResultUnknown,
			fmt.Sprintf("%s: %s", fmt.Sprintf(format, args...), in.Error()), "")
	} else {
		out.Message = fmt.Sprintf("%s: %s", fmt.Sprintf(format, args...), out.Message)
	}
	return out
}

func CombineErrors(errorList ...error) (retErr error) {
	for _, err := range errorList {
		if err != nil {
			if retErr != nil {
				retErr = fmt.Errorf("%v, %v", retErr, err)
			} else {
				retErr = err
			}
		}
	}
	return retErr
}

func GenerateFunctionErrorWithRollback(functionErr, rollbackErr error) error {
	if functionErr != nil {
		if rollbackErr != nil {
			return NewError(ErrorCodeFunctionFailedRollbackFailed,
				functionErr.Error(), rollbackErr.Error())
		}
		return NewError(ErrorCodeFunctionFailedRollbackSucceeded,
			functionErr.Error(), "")
	}
	if rollbackErr != nil {
		return NewError(ErrorCodeResultUnknown,
			fmt.Sprintf("function succeeds but there is rollback error: %v", rollbackErr), "")
	}

	return nil
}

func UnmarshalGRPCError(err error) error {
	st, ok := status.FromError(err)
	if !ok {
		return NewError(ErrorCodeResultUnknown,
			fmt.Sprintf("the error is not a gRPC error: %v", err), "")
	}

	var retErr *Error
	if jsonErr := json.Unmarshal([]byte(st.Message()), &retErr); jsonErr != nil {
		return NewError(ErrorCodeResultUnknown,
			fmt.Sprintf("failed to unmarshal gRPC error message, gRPC err: %v, json error: %v", err, jsonErr), "")
	}
	return retErr
}
