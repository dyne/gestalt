
build:
	cd frontend && npm install && npm run build

test:
	go test ./...
	node --test ./frontend/tests/notificationStore.test.js

clean:
	rm -rf frontend/dist
	rm -rf .cache
