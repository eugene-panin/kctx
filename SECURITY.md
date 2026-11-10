# Security policy

## Supported versions

Only the latest commit on `main` is supported.

## Reporting a vulnerability

`kctx` reads and writes kubeconfig files, which often contain credentials.
Please **do not** open a public issue for security problems.

Email **e.panin@gmail.com** with:

- a description of the issue
- steps to reproduce
- impact (credential exposure, unexpected writes, etc.)

You should get an acknowledgement within a few days. Please give time to fix
and release before any public disclosure.
