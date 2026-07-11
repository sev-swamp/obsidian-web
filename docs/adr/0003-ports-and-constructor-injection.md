# ADR-0003: Ports in core + explicit constructor injection (no DI container)

- Status: accepted
- Date: 2026-07-11

## Context

The spec demands Clean Architecture, SOLID, DI, replaceable
implementations and independently evolving modules. Go offers DI
frameworks (wire, fx, dig) as well as plain constructor wiring.

## Decision

- Every capability is an interface ("port") declared in
  `packages/core/ports.go`: `VaultFS`, `Renderer`, `LinkIndex`,
  `SearchIndex`, `TemplateEngine`, `EventBus`.
- Adapter packages implement the ports and may depend on `core`;
  `core` depends on nothing but the standard library and `shared`.
- Wiring happens by hand in the composition roots (`apps/server`,
  `apps/cli`) with plain constructors.

## Rationale

- Compile-time safety: a missing dependency is a build error, not a
  runtime container panic.
- The dependency graph is small (≈10 components); a container would add
  reflection magic without removing real complexity.
- Two composition roots (server, CLI) already prove the core is
  reusable without any transport.

## Consequences

- Adding a dependency means touching the composition root — acceptable
  and explicit.
- If the graph ever grows unwieldy, google/wire can generate the same
  wiring without changing this architecture.
