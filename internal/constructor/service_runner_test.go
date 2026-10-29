package constructor

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/cloudwego/kitex/pkg/kerrors"
)

// 普通错误不是 Kitex 错误，应原样返回。
func TestUnwrapKitexErrorKeepsNonKitexError(t *testing.T) {
	err := io.EOF

	got := unwrapKitexError(err)

	if got != err {
		t.Fatalf("unwrapKitexError(%v) = %v, want 原样返回 %v", err, got, err)
	}
}

// 非目标错误类型（如 ErrInternalException）必须立即终止并原样返回，
// 不能残留 Kitex 包装，也不能重复解包。
func TestUnwrapKitexErrorStopsForNonTargetErrorType(t *testing.T) {
	err := kerrors.ErrNoConnection

	got := unwrapKitexError(err)

	if got != err {
		t.Fatalf("unwrapKitexError(%v) = %v, want 原样返回 %v", err, got, err)
	}
}

// ErrBiz 类型的包装错误应解包到其 cause。
func TestUnwrapKitexErrorUnwrapsBizErrorCause(t *testing.T) {
	err := kerrors.ErrBiz.WithCause(io.EOF)

	got := unwrapKitexError(err)

	if !errors.Is(got, io.EOF) {
		t.Fatalf("unwrapKitexError(%v) = %v, want 根因 %v", err, got, io.EOF)
	}
	if errors.Is(got, kerrors.ErrBiz) {
		t.Fatalf("unwrapKitexError(%v) = %v, 不应再保留 Kitex 业务错误包装", err, got)
	}
}

// 多层 Kitex 包装应逐层解包到最内层根因。
func TestUnwrapKitexErrorUnwrapsNestedDetailedErrors(t *testing.T) {
	err := kerrors.ErrBiz.WithCause(kerrors.ErrRemoteOrNetwork.WithCause(io.EOF))

	got := unwrapKitexError(err)

	if !errors.Is(got, io.EOF) {
		t.Fatalf("unwrapKitexError(%v) = %v, want 根因 %v", err, got, io.EOF)
	}
}

// cause 为 nil 时解包无进展，必须保留当前错误而不是返回 nil，否则会丢失错误。
func TestUnwrapKitexErrorKeepsErrorWhenCauseIsNil(t *testing.T) {
	err := kerrors.ErrBiz.WithCause(nil)

	got := unwrapKitexError(err)

	if got != err {
		t.Fatalf("unwrapKitexError(%v) = %v, want 原样返回 %v", err, got, err)
	}
}

// 错误处理器对外必须返回原始错误，保留 Kitex 错误分类与业务状态码。
func TestServerErrorHandlerKeepsOriginalError(t *testing.T) {
	handler := newServerErrorHandler("test-service")
	err := kerrors.ErrNoConnection

	got := handler(context.Background(), err)

	if got != err {
		t.Fatalf("handler 返回 %v, want 原始错误 %v", got, err)
	}
}

// 错误处理器不应把 nil 错误变成非 nil 错误。
func TestServerErrorHandlerReturnsNilForNilError(t *testing.T) {
	handler := newServerErrorHandler("test-service")

	if got := handler(context.Background(), nil); got != nil {
		t.Fatalf("handler(nil) = %v, want nil", got)
	}
}
