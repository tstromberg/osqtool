// osqtool operates on osquery query and pack files
//
// Copyright 2021 Chainguard, Inc.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"chainguard.dev/osqtool/pkg/query"
	"github.com/hashicorp/go-multierror"

	"k8s.io/klog/v2"
)

func main() {
	outputFlag := flag.String("output", "", "Location of output")

	flag.Parse()
	args := flag.Args()

	if len(args) < 2 {
		klog.Exitf("usage: osqtool [pack|unpack] <path>")
	}

	action := args[0]
	path := args[1]
	var err error

	switch action {
	case "pack":
		err = Pack(path, *outputFlag)
	case "unpack":
		err = Unpack(path, *outputFlag)
	case "verify":
		err = Verify(path)
	default:
		err = fmt.Errorf("unknown action")
	}
	if err != nil {
		klog.Exitf("%q failed: %v", action, err)
	}
}

func Pack(sourcePath string, output string) error {
	mm, err := query.LoadFromDir(sourcePath)
	if err != nil {
		return fmt.Errorf("load from dir: %v", err)
	}

	bs, err := query.RenderPack(mm)
	if err != nil {
		return fmt.Errorf("render: %v", err)
	}

	if output == "" {
		_, err = fmt.Println(string(bs))
		return err
	}

	return os.WriteFile(output, bs, 0o600)
}

func Unpack(sourcePath string, destPath string) error {
	if destPath == "" {
		destPath = "."
	}

	p, err := query.LoadPack(sourcePath)
	if err != nil {
		return fmt.Errorf("load pack: %v", err)
	}

	err = query.SaveToDirectory(p.Queries, destPath)
	if err != nil {
		return fmt.Errorf("save to dir: %v", err)
	}

	fmt.Printf("%d queries saved to %s\n", len(p.Queries), destPath)
	return nil
}

func Verify(path string) error {
	s, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}

	mm := map[string]*query.Metadata{}

	switch {
	case s.IsDir():
		mm, err = query.LoadFromDir(path)
		if err != nil {
			return fmt.Errorf("load from dir: %w", err)
		}
	case strings.HasSuffix(path, ".conf"):
		p, err := query.LoadPack(path)
		if err != nil {
			return fmt.Errorf("load from dir: %w", err)
		}
		mm = p.Queries
	default:
		m, err := query.Load(path)
		if err != nil {
			return fmt.Errorf("load: %w", err)
		}
		mm[m.Name] = m
	}

	verified := 0
	skipped := 0
	errored := 0

	for name, m := range mm {
		klog.Infof("Verifying %q ...", name)
		vf, verr := query.Verify(m)
		if verr != nil {
			klog.Errorf("%q failed validation: %v", name, verr)
			err = multierror.Append(err, fmt.Errorf("%s: %w", name, verr))
			errored++
			continue
		}

		if vf.IncompatiblePlatform != "" {
			klog.Warningf("Skipped %q: incompatible platform: %q", name, vf.IncompatiblePlatform)
			skipped++
			continue
		}

		klog.Infof("%q returned %d rows within %s", name, len(vf.Results), vf.Elapsed)
		verified++
	}

	klog.Infof("%d queries found: %d verified, %d errored, %d skipped", len(mm), verified, errored, skipped)

	if verified == 0 {
		err = multierror.Append(err, fmt.Errorf("0 queries were verified"))
	}

	return err
}
