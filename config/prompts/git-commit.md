On git commit of any changes, use conventional commit messages and
notify:
gestalt-notify --port {{port backend}} --session-id {{session id}} '{"type":"git-commit","git-branch":"...","commit-hash":"...","commit-message":"..."}'
