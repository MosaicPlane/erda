package main

import (
	"fmt"

	_ "github.com/erda-project/erda-proto-go"
	_ "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func main() {
	count := 0
	protoregistry.GlobalFiles.RangeFiles(func(_ protoreflect.FileDescriptor) bool {
		count++
		return true
	})
	if count == 0 {
		panic("global protobuf registry is empty")
	}
	fmt.Printf("registered-proto-files=%d\n", count)
}
