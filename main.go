package main

import (
	"flag"
	"fmt"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/pluginpb"
)

const version = "0.0.1"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("protoc-gen-openapi %v\n", version)
		return
	}

	var flags flag.FlagSet

	protogen.Options{
		ParamFunc: flags.Set,
	}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		msg := make(map[string]*protogen.Message)
		for _, f := range gen.Files {
			for _, m := range f.Messages {
				msg[string(m.Desc.FullName())] = m
			}
			if !f.Generate {
				continue
			}
			generateFile(gen, f, msg)
		}
		return nil
	})
}
