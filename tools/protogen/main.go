// protogen: a tiny protoc replacement. Compiles .proto files with
// bufbuild/protocompile and runs protoc plugins (protoc-gen-go, …) exactly
// as protoc would: CodeGeneratorRequest on stdin, CodeGeneratorResponse out.
//
// usage: protogen -I <import dir> -plugin name=path[:param] ... -out <dir> file.proto ...
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	var includes, plugins multi
	flag.Var(&includes, "I", "import path")
	flag.Var(&plugins, "plugin", "name=path[:param]")
	out := flag.String("out", ".", "output dir")
	flag.Parse()
	files := flag.Args()

	comp := protocompile.Compiler{
		Resolver:       protocompile.WithStandardImports(&protocompile.SourceResolver{ImportPaths: includes}),
		SourceInfoMode: protocompile.SourceInfoStandard,
	}
	res, err := comp.Compile(context.Background(), files...)
	if err != nil {
		fail(err)
	}

	// All files in dependency order (deps first), as protoc sends them.
	var all []*descriptorpb.FileDescriptorProto
	seen := map[string]bool{}
	var add func(fd protoreflect.FileDescriptor)
	add = func(fd protoreflect.FileDescriptor) {
		if seen[fd.Path()] {
			return
		}
		seen[fd.Path()] = true
		imps := fd.Imports()
		for i := 0; i < imps.Len(); i++ {
			add(imps.Get(i).FileDescriptor)
		}
		all = append(all, protodesc.ToFileDescriptorProto(fd))
	}
	for _, f := range res {
		add(f)
	}

	for _, p := range plugins {
		name, rest, _ := strings.Cut(p, "=")
		path, param, _ := strings.Cut(rest, ":")
		req := &pluginpb.CodeGeneratorRequest{FileToGenerate: files, ProtoFile: all}
		if param != "" {
			req.Parameter = proto.String(param)
		}
		in, _ := proto.Marshal(req)
		cmd := exec.Command(path)
		cmd.Stdin = bytes.NewReader(in)
		var stdout bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fail(fmt.Errorf("%s: %w", name, err))
		}
		var resp pluginpb.CodeGeneratorResponse
		if err := proto.Unmarshal(stdout.Bytes(), &resp); err != nil {
			fail(err)
		}
		if resp.Error != nil {
			fail(fmt.Errorf("%s: %s", name, resp.GetError()))
		}
		for _, f := range resp.File {
			dst := filepath.Join(*out, f.GetName())
			os.MkdirAll(filepath.Dir(dst), 0o755)
			if err := os.WriteFile(dst, []byte(f.GetContent()), 0o644); err != nil {
				fail(err)
			}
			fmt.Println("wrote", dst)
		}
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, "protogen:", err); os.Exit(1) }
