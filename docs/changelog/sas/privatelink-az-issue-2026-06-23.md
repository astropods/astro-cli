# PrivateLink: select only AZ-compatible subnets per endpoint service

## Summary

Creating an Interface VPC Endpoint for a knowledge store passed the full
`PRIVATELINK_SUBNET_IDS` list to `CreateVpcEndpoint`. AWS rejects the entire
call with `InvalidParameter: ... does not support the availability zone of the
subnet` if *any* passed subnet sits in an AZ the target endpoint service is not
published in. With one subnet per AZ now spanning all six us-east-1 AZs, any
service not published in all six broke provisioning.

The provision worker now selects, per target service, only the subnets whose AZ
the service actually supports.

## Design

Before `CreateVpcEndpoint`, the worker resolves the AZ intersection:

- `DescribeVpcEndpointServices` for the target service → `ServiceDetails[0].AvailabilityZones`.
  These are mapped into our consumer-account AZ naming, so they compare directly
  to our subnets' AZ names (no AZ-ID resolution).
- `DescribeSubnets` over the whole `PRIVATELINK_SUBNET_IDS` list once, building a
  subnet→AZ map cached on the worker (a subnet's AZ never changes).
- Pass only the subnets whose AZ is in the service's set, preserving configured
  order.
- Empty intersection → fail fast with an actionable error naming the service and
  both AZ sets, instead of surfacing the raw AWS error.

Nothing is hardcoded — the AZ list and subnet count derive entirely from the env
var and the two describe calls, so the set can grow without code changes.
Placing endpoints in only the supported AZs (typically 1–3) also keeps ENI usage
well under the per-region limit.

## Migration

Requires two additional IAM actions on the astro-server IRSA role:
`ec2:DescribeVpcEndpointServices` and `ec2:DescribeSubnets`. These are added in
the infra repo separately; without them the new describe calls fail with
`AccessDenied`. No user-facing migration.
