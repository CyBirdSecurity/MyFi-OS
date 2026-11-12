# Contributing to MyFi

Thanks for helping improve MyFi. Bug reports, fixes and focused improvements are all welcome.

## Reporting bugs and ideas

Open an issue describing what happened, what you expected, and how to reproduce it. For security problems, follow [SECURITY.md](SECURITY.md) instead of opening an issue.

## Making a change

1. Fork the repository and create a branch.
2. Set up the development environment as described under [Development](README.md#development) in the README.
3. Make your change. Keep it focused on one thing, and match the style of the surrounding code.
4. Add or update tests for behavior you change.
5. Run the checks:

   ```bash
   gofmt -l .               # should print nothing
   go vet ./...
   make test                # Go tests
   npm --prefix web test    # frontend tests
   npm --prefix web run build
   ```

6. Open a pull request explaining what changed and why.

Database changes go in a new numbered migration pair under `migrations/`. Don't edit an existing migration.

MyFi is a self-hosted, single-account app. Changes that add required external services or accounts are unlikely to be accepted; optional integrations should keep MyFi fully usable without them.

## License

By contributing, you agree that your contributions are licensed under the [MIT License](LICENSE).
