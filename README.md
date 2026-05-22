# cluster-autoscaler-agent

Bootstrapped project for building a tool harness that can emulate Kubernetes Cluster Autoscaler interactions.

## First tool: `externalgrpcctl`

`externalgrpcctl` is a CLI for exercising an external gRPC cloud provider implementation with the same call patterns used by Cluster Autoscaler.

### Supported commands

- `list-nodegroups` → calls `NodeGroups`
- `template --nodegroup <id>` → calls `NodeGroupTemplateNodeInfo`
- `node-info --nodegroup <id>` → calls `NodeGroupNodes`
- `scale-up --nodegroup <id> --delta <n>` → calls `IncreaseSize`
- `scale-down --nodegroup <id> --delta <n>` → calls `DecreaseTargetSize` (negative delta)

### Usage

```bash
go run ./cmd/externalgrpcctl --addr 127.0.0.1:5200 list-nodegroups
go run ./cmd/externalgrpcctl --addr 127.0.0.1:5200 template --nodegroup ng-1
go run ./cmd/externalgrpcctl --addr 127.0.0.1:5200 node-info --nodegroup ng-1
go run ./cmd/externalgrpcctl --addr 127.0.0.1:5200 scale-up --nodegroup ng-1 --delta 2
go run ./cmd/externalgrpcctl --addr 127.0.0.1:5200 scale-down --nodegroup ng-1 --delta 1
```

Optional headers are supported with repeated `-H key:value` flags.

## Nix dev environment

A `flake.nix` is included for reproducible tooling.

```bash
nix develop
```

Includes Go, gopls, golangci-lint, grpcurl, protobuf, buf, and just.
