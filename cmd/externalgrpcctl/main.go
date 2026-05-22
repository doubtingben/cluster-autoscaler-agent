package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fullstorydev/grpcurl"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const externalProviderService = "clusterautoscaler.ExternalGrpcProvider"

type config struct {
	address string
	timeout time.Duration
	headers multiFlag
}

type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.address, "addr", "127.0.0.1:5200", "gRPC provider address")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Second, "request timeout")
	flag.Var(&cfg.headers, "H", "metadata header key:value (repeatable)")
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		usage()
		os.Exit(2)
	}

	svc, method, reqBody, err := buildCall(args)
	if err != nil {
		fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	ctx, err = withOutgoingHeaders(ctx, cfg.headers)
	if err != nil {
		fatal(err)
	}

	cc, err := grpc.DialContext(ctx, cfg.address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fatal(err)
	}
	defer cc.Close()

	rc := grpcreflect.NewClientAuto(ctx, cc)
	defer rc.Reset()

	desc := grpcurl.DescriptorSourceFromServer(ctx, rc)
	if err := invoke(ctx, cc, desc, svc+"."+method, reqBody); err != nil {
		fatal(err)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `externalgrpcctl: call Cluster Autoscaler external gRPC provider endpoints

Usage:
  externalgrpcctl [global flags] <command> [command flags]

Commands:
  list-nodegroups
  template --nodegroup <id>
  node-info --nodegroup <id>
  scale-up --nodegroup <id> --delta <n>
  scale-down --nodegroup <id> --delta <n>

Global flags:
`)
	flag.PrintDefaults()
}

func buildCall(args []string) (service string, method string, reqBody string, err error) {
	if len(args) == 0 {
		return "", "", "", errors.New("missing command")
	}

	switch args[0] {
	case "list-nodegroups":
		return externalProviderService, "NodeGroups", `{}`, nil
	case "template":
		fs := newFlagSet("template")
		nodegroup := fs.String("nodegroup", "", "nodegroup ID")
		if err := fs.Parse(args[1:]); err != nil {
			return "", "", "", err
		}
		if strings.TrimSpace(*nodegroup) == "" {
			return "", "", "", errors.New("template requires --nodegroup")
		}
		return externalProviderService, "NodeGroupTemplateNodeInfo", marshal(map[string]any{"id": *nodegroup}), nil
	case "node-info":
		fs := newFlagSet("node-info")
		nodegroup := fs.String("nodegroup", "", "nodegroup ID")
		if err := fs.Parse(args[1:]); err != nil {
			return "", "", "", err
		}
		if strings.TrimSpace(*nodegroup) == "" {
			return "", "", "", errors.New("node-info requires --nodegroup")
		}
		return externalProviderService, "NodeGroupNodes", marshal(map[string]any{"id": *nodegroup}), nil
	case "scale-up", "scale-down":
		fs := newFlagSet(args[0])
		nodegroup := fs.String("nodegroup", "", "nodegroup ID")
		delta := fs.Int("delta", 0, "change in target size")
		if err := fs.Parse(args[1:]); err != nil {
			return "", "", "", err
		}
		if strings.TrimSpace(*nodegroup) == "" || *delta <= 0 {
			return "", "", "", fmt.Errorf("%s requires --nodegroup and --delta > 0", args[0])
		}
		if args[0] == "scale-down" {
			return externalProviderService, "DecreaseTargetSize", marshal(map[string]any{"id": *nodegroup, "delta": -*delta}), nil
		}
		return externalProviderService, "IncreaseSize", marshal(map[string]any{"id": *nodegroup, "delta": *delta}), nil
	default:
		return "", "", "", fmt.Errorf("unknown command %q", args[0])
	}
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func marshal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func withOutgoingHeaders(ctx context.Context, headers []string) (context.Context, error) {
	if len(headers) == 0 {
		return ctx, nil
	}
	md := metadata.New(nil)
	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid header %q, expected key:value", h)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("invalid header %q, empty key", h)
		}
		md.Append(key, value)
	}
	return metadata.NewOutgoingContext(ctx, md), nil
}

func invoke(ctx context.Context, cc *grpc.ClientConn, src grpcurl.DescriptorSource, method string, reqBody string) error {
	parser, formatter, err := grpcurl.RequestParserAndFormatter(grpcurl.FormatJSON, src, strings.NewReader(reqBody), grpcurl.FormatOptions{})
	if err != nil {
		return err
	}
	h := grpcurl.NewDefaultEventHandler(os.Stdout, src, formatter, false)
	return grpcurl.InvokeRPC(ctx, src, cc, method, nil, h, parser.Next)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
