# cluster-autoscaler-agent

Bootstrapped project for building a tool harness that can emulate Kubernetes Cluster Autoscaler interactions.

## First tool: `externalgrpcctl`

`externalgrpcctl` is a CLI for exercising an external gRPC cloud provider implementation with the same call patterns used by Cluster Autoscaler.

`list-nodegroups` enriches the base `NodeGroups` response with live `currentSize` and `desiredSize` values by also querying `NodeGroupNodes` and `NodeGroupTargetSize` for each group.

### Supported commands

- `list-nodegroups` → calls `NodeGroups`
- `template --nodegroup <id>` → calls `NodeGroupTemplateNodeInfo`
- `node-info --nodegroup <id>` → calls `NodeGroupNodes`
- `scale-up --nodegroup <id> --delta <n>` → calls `IncreaseSize`
- `node-delete --nodegroup <id> --node <name>` → calls `NodeGroupDeleteNodes`
- `node-delete --nodegroup <id> --provider-id <id>` → calls `NodeGroupDeleteNodes`

### Usage

```bash
go run ./cmd/externalgrpcctl --addr 127.0.0.1:8086 list-nodegroups
go run ./cmd/externalgrpcctl --addr 127.0.0.1:8086 template --nodegroup ng-1
go run ./cmd/externalgrpcctl --addr 127.0.0.1:8086 node-info --nodegroup ng-1
go run ./cmd/externalgrpcctl --addr 127.0.0.1:8086 scale-up --nodegroup ng-1 --delta 2
go run ./cmd/externalgrpcctl --addr 127.0.0.1:8086 node-delete --nodegroup ng-1 --node worker-a
go run ./cmd/externalgrpcctl --addr 127.0.0.1:8086 node-delete --nodegroup ng-1 --provider-id aws:///us-east-1a/i-1234567890
```

Optional headers are supported with repeated `-H key:value` flags.

## Nix dev environment

A `flake.nix` is included for reproducible tooling.

```bash
nix develop
```

Includes Go, gopls, golangci-lint, grpcurl, protobuf, buf, and just.
