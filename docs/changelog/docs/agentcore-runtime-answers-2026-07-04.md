# AgentCore runtime notes — AWS answers

## Summary

The AgentCore integration note (`docs/05-architecture/agentcore-runtime.md`) captured nine open questions we had put to the AWS Bedrock AgentCore team about running Astro agents on AgentCore Runtime while keeping the rest of the platform on EKS. AWS has now answered all nine. This folds their responses into the doc so it reads as a resolved reference rather than an outstanding ask.

## Design

The "Open questions for the AgentCore team" section becomes "Answers from the AgentCore team." Each item now pairs a short restatement of our question with AWS's answer, preserving the load-bearing specifics: VPC network mode (ENIs in our subnets) as the mechanism for private reach, hard runtime limits (15 min sync / 60 min streaming / 8 h async / 1 GB session storage), microVM session lifecycle for persistent outbound connections, PrivateLink endpoint names and IAM condition keys for private inbound invoke, layered egress controls, security-group-based egress identity, and the Backend-for-Frontend pattern for customer-facing web UIs. The top-line description and References section are updated to match; AWS-cited docs are listed by title only since AWS supplied link text rather than URLs.

The net finding: the crux — reaching a private VPC from the runtime — is confirmed supported, so the integration model in the doc stands.

## Migration

None. Documentation only.
