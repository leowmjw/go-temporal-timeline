run:
	@go run *.go

test:
	@gotest ./...

watch:
	@pkgx watch --color jj --ignore-working-copy log --color=always

detailed:
	@jj log -T builtin_log_detailed

evolog:
	@jj evolog -p

squash:
	@jj squash

new:
	@jj new main

push:
	@echo "Random new branch for main .."
	@jj git push

