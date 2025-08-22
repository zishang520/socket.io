# Security Policy

## Supported Versions

The following versions of Socket.IO Go implementation are currently being supported with security updates:

| Version | Supported          |
| ------- | ------------------ |
| 3.x.x   | :white_check_mark: |
| 2.x.x   | :white_check_mark: |
| 1.x.x   | :x:                |

## Reporting a Vulnerability

We take the security of Socket.IO Go implementation seriously. If you believe you have found a security vulnerability, please follow these steps:

1. **Do Not** disclose the vulnerability publicly.
2. Submit a report through one of these channels:
   - Open a [security advisory](https://github.com/zishang520/socket.io/security/advisories/new)
   - Send an email to [maintainer's email] with details of the issue

Please include the following information in your report:

- A clear description of the vulnerability
- Steps to reproduce the issue
- Affected versions
- Potential impact
- Any possible mitigations

## Response Timeline

- Initial response: within 48 hours
- Status update: within 5 business days
- Security patch: timeline will vary based on severity and complexity

## Security Update Process

1. The security team will acknowledge receipt of your vulnerability report
2. We will investigate and validate the issue
3. We will develop and test a fix
4. A security advisory will be published once the fix is ready
5. The fix will be deployed to all supported versions

## Best Practices

When using Socket.IO in your applications, consider these security best practices:

1. Always use the latest stable version
2. Implement proper authentication mechanisms
3. Use secure WebSocket connections (wss://)
4. Configure CORS policies appropriately
5. Regularly update dependencies

## Public Disclosure

Security vulnerabilities will be disclosed via:

1. GitHub Security Advisories
2. Release notes
3. The official Socket.IO Go security mailing list (if applicable)

## Contact

For security-related inquiries, contact:

- GitHub Security Advisory