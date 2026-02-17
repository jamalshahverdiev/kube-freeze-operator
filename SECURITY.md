# Security Policy

## Supported Versions

We release patches for security vulnerabilities. Which versions are eligible for receiving such patches depends on the CVSS v3.0 Rating:

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

If you discover a security vulnerability in kube-freeze-operator, please report it to us privately:

### How to Report

1. **Email**: Send details to the maintainers through the project's private communication channel
2. **Issue Tracker**: Create a security advisory through GitHub's security advisory feature
3. **GitLab**: Use GitLab's confidential issues feature if reporting through GitLab

### What to Include

Please include the following information:

- Type of vulnerability (e.g., privilege escalation, information disclosure)
- Full paths of affected source files
- Location of the affected source code (tag/branch/commit)
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact of the vulnerability
- Suggested mitigation or fix (if any)

### Response Timeline

- **Initial Response**: We aim to respond within 48 hours
- **Status Update**: We will provide status updates at least every 5 business days
- **Fix Timeline**: Critical vulnerabilities will be addressed within 7 days
- **Public Disclosure**: After a fix is released, we will publicly disclose the vulnerability

## Security Best Practices

When deploying kube-freeze-operator:

### RBAC Permissions

- Review and understand the operator's required RBAC permissions
- Use namespace-scoped deployments when possible
- Limit `FreezeException` creation to trusted users/groups

### Webhook Configuration

- Ensure cert-manager is properly configured for TLS certificates
- Use `failurePolicy: Fail` in production to prevent bypassing freeze policies
- Monitor webhook certificate expiration

### Network Policies

- Apply network policies to restrict operator communication
- Limit ingress/egress to necessary endpoints only

### Audit Logging

- Enable Kubernetes audit logging for freeze policy events
- Monitor denied requests through Prometheus metrics
- Review exception usage regularly

### Access Control

- Restrict access to the operator's namespace
- Use separate service accounts for different components
- Implement Pod Security Standards/Policies

### Updates

- Keep the operator updated to the latest stable version
- Subscribe to security advisories
- Test updates in non-production environments first

## Known Security Considerations

### Admission Webhook Bypass

The operator's admission webhook can be bypassed if:
- Webhook service is unavailable and `failurePolicy: Ignore` is set
- User has permissions to modify ValidatingWebhookConfiguration
- Operator is deployed in the same namespace as workloads

**Mitigation**: Deploy operator in dedicated namespace, use `failurePolicy: Fail`, implement proper RBAC.

### Exception Abuse

`FreezeException` resources can be abused if creation permissions are too broad.

**Mitigation**: Limit `FreezeException` creation to SRE/Platform teams only.

### CronJob Modifications

The operator modifies CronJob `spec.suspend` field, which could conflict with other operators or controllers.

**Mitigation**: Use `managed-by` annotations to prevent conflicts. Review CronJob changes through audit logs.

## Security Updates

Security updates will be published as:
- GitHub Releases with security tags
- Security advisories
- CHANGELOG.md entries marked as security fixes

Subscribe to repository notifications to receive security updates.

## Acknowledgments

We appreciate security researchers and users who report vulnerabilities responsibly. Contributors who report valid security issues will be acknowledged (unless they prefer to remain anonymous).

Thank you for helping keep kube-freeze-operator secure! 🔒
