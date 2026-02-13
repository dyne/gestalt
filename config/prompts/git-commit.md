On git commit of any changes, use conventional commit messages and then run gestalt-notify with the commit:
gestalt-notify --port {{port backend}} --session-id {{session id}} '{"type":"commit","git-branch":"...","commit-hash":"...","commit-message":"..."}'
