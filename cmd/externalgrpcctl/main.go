package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/externalgrpc/protos"
)

type config struct {
	address string
	timeout time.Duration
	headers multiFlag
	tls     bool
}

type rpcCall struct {
	run func(context.Context, protos.CloudProviderClient) (any, error)
}

type nodeGroupSummary struct {
	ID          string `json:"id"`
	MinSize     int32  `json:"minSize"`
	MaxSize     int32  `json:"maxSize"`
	CurrentSize int    `json:"currentSize"`
	DesiredSize int32  `json:"desiredSize"`
	Debug       string `json:"debug"`
}

type listNodeGroupsResponse struct {
	NodeGroups []nodeGroupSummary `json:"nodeGroups"`
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
	flag.StringVar(&cfg.address, "addr", "127.0.0.1:8086", "gRPC provider address")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Second, "request timeout")
	flag.Var(&cfg.headers, "H", "metadata header key:value (repeatable)")
	flag.BoolVar(&cfg.tls, "tls", false, "use TLS to connect to the provider")
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		usage()
		os.Exit(2)
	}

	call, err := buildCall(args)
	if err != nil {
		fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	ctx, err = withOutgoingHeaders(ctx, cfg.headers, cfg.tls)
	if err != nil {
		fatal(err)
	}

	var creds credentials.TransportCredentials
	if cfg.tls {
		creds = credentials.NewTLS(&tls.Config{
			MinVersion: tls.VersionTLS12,
		})
	} else {
		creds = insecure.NewCredentials()
	}

	cc, err := grpc.NewClient(cfg.address, grpc.WithTransportCredentials(creds))
	if err != nil {
		fatal(err)
	}
	defer cc.Close()

	client := protos.NewCloudProviderClient(cc)
	resp, err := call.run(ctx, client)
	if err != nil {
		fatal(err)
	}
	if err := printResponse(resp); err != nil {
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
  node-delete --nodegroup <id> --node <name> [--node <name> ...]
  node-delete --nodegroup <id> --provider-id <id> [--provider-id <id> ...]

Global flags:
`)
	flag.PrintDefaults()
}

func buildCall(args []string) (rpcCall, error) {
	if len(args) == 0 {
		return rpcCall{}, errors.New("missing command")
	}

	switch args[0] {
	case "list-nodegroups":
		return rpcCall{
			run: func(ctx context.Context, client protos.CloudProviderClient) (any, error) {
				resp, err := client.NodeGroups(ctx, &protos.NodeGroupsRequest{})
				if err != nil {
					return nil, err
				}

				summaries := make([]nodeGroupSummary, 0, len(resp.GetNodeGroups()))
				for _, ng := range resp.GetNodeGroups() {
					targetResp, err := client.NodeGroupTargetSize(ctx, &protos.NodeGroupTargetSizeRequest{Id: ng.GetId()})
					if err != nil {
						return nil, fmt.Errorf("nodegroup %q target size: %w", ng.GetId(), err)
					}
					nodesResp, err := client.NodeGroupNodes(ctx, &protos.NodeGroupNodesRequest{Id: ng.GetId()})
					if err != nil {
						return nil, fmt.Errorf("nodegroup %q current nodes: %w", ng.GetId(), err)
					}

					summaries = append(summaries, nodeGroupSummary{
						ID:          ng.GetId(),
						MinSize:     ng.GetMinSize(),
						MaxSize:     ng.GetMaxSize(),
						CurrentSize: len(nodesResp.GetInstances()),
						DesiredSize: targetResp.GetTargetSize(),
						Debug:       ng.GetDebug(),
					})
				}

				return listNodeGroupsResponse{NodeGroups: summaries}, nil
			},
		}, nil
	case "template":
		fs := newFlagSet("template")
		nodegroup := fs.String("nodegroup", "", "nodegroup ID")
		if err := fs.Parse(args[1:]); err != nil {
			return rpcCall{}, err
		}
		if strings.TrimSpace(*nodegroup) == "" {
			return rpcCall{}, errors.New("template requires --nodegroup")
		}
		return rpcCall{
			run: func(ctx context.Context, client protos.CloudProviderClient) (any, error) {
				return client.NodeGroupTemplateNodeInfo(ctx, &protos.NodeGroupTemplateNodeInfoRequest{Id: *nodegroup})
			},
		}, nil
	case "node-info":
		fs := newFlagSet("node-info")
		nodegroup := fs.String("nodegroup", "", "nodegroup ID")
		if err := fs.Parse(args[1:]); err != nil {
			return rpcCall{}, err
		}
		if strings.TrimSpace(*nodegroup) == "" {
			return rpcCall{}, errors.New("node-info requires --nodegroup")
		}
		return rpcCall{
			run: func(ctx context.Context, client protos.CloudProviderClient) (any, error) {
				return client.NodeGroupNodes(ctx, &protos.NodeGroupNodesRequest{Id: *nodegroup})
			},
		}, nil
	case "scale-up":
		fs := newFlagSet(args[0])
		nodegroup := fs.String("nodegroup", "", "nodegroup ID")
		delta := fs.Int("delta", 0, "change in target size")
		if err := fs.Parse(args[1:]); err != nil {
			return rpcCall{}, err
		}
		if strings.TrimSpace(*nodegroup) == "" || *delta <= 0 {
			return rpcCall{}, fmt.Errorf("%s requires --nodegroup and --delta > 0", args[0])
		}
		if *delta > math.MaxInt32 {
			return rpcCall{}, fmt.Errorf("%s --delta exceeds maximum allowed value (math.MaxInt32)", args[0])
		}
		return rpcCall{
			run: func(ctx context.Context, client protos.CloudProviderClient) (any, error) {
				return client.NodeGroupIncreaseSize(ctx, &protos.NodeGroupIncreaseSizeRequest{
					Id:    *nodegroup,
					Delta: int32(*delta), // #nosec G115 -- bounded by math.MaxInt32 check above
				})
			},
		}, nil
	case "node-delete":
		fs := newFlagSet("node-delete")
		nodegroup := fs.String("nodegroup", "", "nodegroup ID")
		var nodeNames multiFlag
		var providerIDs multiFlag
		fs.Var(&nodeNames, "node", "Kubernetes node name to delete (repeatable)")
		fs.Var(&providerIDs, "provider-id", "cloud provider node ID to delete (repeatable)")
		if err := fs.Parse(args[1:]); err != nil {
			return rpcCall{}, err
		}
		if strings.TrimSpace(*nodegroup) == "" {
			return rpcCall{}, errors.New("node-delete requires --nodegroup")
		}
		nodes := make([]*protos.ExternalGrpcNode, 0, len(nodeNames)+len(providerIDs))
		for _, name := range nodeNames {
			name = strings.TrimSpace(name)
			if name == "" {
				return rpcCall{}, errors.New("node-delete received empty --node value")
			}
			nodes = append(nodes, &protos.ExternalGrpcNode{Name: name})
		}
		for _, providerID := range providerIDs {
			providerID = strings.TrimSpace(providerID)
			if providerID == "" {
				return rpcCall{}, errors.New("node-delete received empty --provider-id value")
			}
			nodes = append(nodes, &protos.ExternalGrpcNode{ProviderID: providerID})
		}
		if len(nodes) == 0 {
			return rpcCall{}, errors.New("node-delete requires at least one --node or --provider-id")
		}
		return rpcCall{
			run: func(ctx context.Context, client protos.CloudProviderClient) (any, error) {
				return client.NodeGroupDeleteNodes(ctx, &protos.NodeGroupDeleteNodesRequest{
					Id:    *nodegroup,
					Nodes: nodes,
				})
			},
		}, nil
	default:
		return rpcCall{}, fmt.Errorf("unknown command %q", args[0])
	}
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func withOutgoingHeaders(ctx context.Context, headers []string, tlsEnabled bool) (context.Context, error) {
	if len(headers) == 0 {
		return ctx, nil
	}
	md := metadata.New(nil)
	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			// SECURITY: Return a generic error message to avoid logging sensitive data (like tokens)
			// from a malformed header 'h' that a user might have accidentally provided.
			return nil, errors.New("invalid header format, expected key:value")
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			// SECURITY: Return a generic error message to avoid logging sensitive data.
			return nil, errors.New("invalid header format, empty key")
		}
		keyLower := strings.ToLower(key)
		if !tlsEnabled && (keyLower == "authorization" || keyLower == "cookie" || strings.Contains(keyLower, "token") || strings.Contains(keyLower, "secret") || strings.Contains(keyLower, "key") || strings.Contains(keyLower, "password") || strings.Contains(keyLower, "credential") || strings.Contains(keyLower, "auth")) {
			// SECURITY: Prevent transmitting credentials or sensitive tokens in plaintext
			return nil, fmt.Errorf("SECURITY: refusing to send sensitive header %q over an unencrypted connection (requires --tls)", key)
		}
		valueLower := strings.ToLower(value)
		if !tlsEnabled && (strings.HasPrefix(valueLower, "bearer ") || strings.HasPrefix(valueLower, "ghp_") || strings.HasPrefix(valueLower, "glpat-")) {
			// SECURITY: Prevent transmitting known token formats in plaintext regardless of header key
			return nil, fmt.Errorf("SECURITY: refusing to send token-like value in header %q over an unencrypted connection (requires --tls)", key)
		}
		md.Append(key, value)
	}
	return metadata.NewOutgoingContext(ctx, md), nil
}

func printResponse(v any) error {
	if templateResp, ok := v.(*protos.NodeGroupTemplateNodeInfoResponse); ok {
		return printTemplateNode(templateResp)
	}
	if msg, ok := v.(proto.Message); ok {
		out, err := protojson.MarshalOptions{
			Multiline:       true,
			Indent:          "  ",
			EmitUnpopulated: true,
		}.Marshal(msg)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(os.Stdout, string(out))
		return err
	}

	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(out))
	return err
}

func printTemplateNode(resp *protos.NodeGroupTemplateNodeInfoResponse) error {
	node := &corev1.Node{}
	if err := node.Unmarshal(resp.GetNodeBytes()); err != nil {
		return fmt.Errorf("decode template nodeBytes: %w", err)
	}

	out, err := json.MarshalIndent(node, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(out))
	return err
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
