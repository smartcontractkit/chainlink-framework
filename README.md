# chainlink-framework

This repo contains common components created and maintained by the Blockchain Integrations Framework team.
These components are used across EVM and non-EVM chain integrations.

## Components

### MultiNode
Enables the use of multiple RPCs in chain integrations. Performs critical health checks, 
load balancing, node metrics, and is used to send transactions to all RPCs and aggregate results.
MultiNode is used by all other components which require reading from or writing to the chain.

