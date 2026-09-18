package httpapi

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMultipartRequestHasHardLimit(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "large.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write(bytes.Repeat([]byte("x"), 2048)); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/upload/sdcard/large.bin", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	err = parseMultipartUploadLimit(httptest.NewRecorder(), request, 1024)
	var tooLarge *http.MaxBytesError
	if !errors.As(err, &tooLarge) || multipartErrorStatus(err) != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected request-too-large error, got %T %v", err, err)
	}
}

func TestUploadLimitMapsToRequestEntityTooLarge(t *testing.T) {
	err := errors.New("upload exceeds 536870912 byte limit")
	if status := httpStatusForFile(err); status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d", status)
	}
}
