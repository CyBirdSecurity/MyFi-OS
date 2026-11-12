# Security Policy

MyFi stores personal financial data, so security reports are taken seriously.

## Reporting a vulnerability

Please **don't open a public issue** for security problems. Report them privately through GitHub instead: open the repository's **Security** tab and choose **Report a vulnerability**.

Include what you found, how to reproduce it, and the version or commit you tested. You'll get an acknowledgement within a few days, and a fix or a plan once the issue is confirmed. Please give us a reasonable time to release a fix before disclosing it publicly.

## Supported versions

Security fixes go into the latest release. Upgrade with `git pull && docker compose up -d --build` to stay current.

## Running MyFi safely

- Keep `.env` private and generate `POSTGRES_PASSWORD`, `JWT_SECRET` and `MYFI_ENCRYPTION_KEY` with `openssl rand -hex 32`.
- Don't publish PostgreSQL or MyFi's port directly to the internet. Put an HTTPS reverse proxy in front and set `SECURE_COOKIES=true`.
- Back up your database and keep backups as private as the data itself.
