OCB_VER := 0.144.0

download-otel:
	$(info Downloading Open Telemetry Builder)
	@[ -r ocb ] || curl --proto '=https' --tlsv1.2 -fL -o ocb https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/cmd%2Fbuilder%2Fv${OCB_VER}/ocb_${OCB_VER}_${OS}_${ARCH}
	@chmod +x ocb

Linux_x86_64: OS := linux
Linux_x86_64: ARCH := amd64
Linux_x86_64: download-otel
	$(info Open Telemetry Build)
	@./ocb --config builder-config.yaml
	@mv gestalt-otel ..

Linux_arm64: OS := linux
Linux_arm64: ARCH := arm64
Linux_arm64: download-otel
	$(info Open Telemetry Build)
	@./ocb --config builder-config.yaml
	@mv gestalt-otel ..

Linux_aarch64: Linux_arm64

Darwin_x86_64: OS := darwin
Darwin_x86_64: ARCH := amd64
Darwin_x86_64: download-otel
	$(info Open Telemetry Build)
	@./ocb --config builder-config.yaml
	@mv gestalt-otel ..

Darwin_arm64: OS := darwin
Darwin_arm64: ARCH := arm64
Darwin_arm64: download-otel
	$(info Open Telemetry Build)
	@./ocb --config builder-config.yaml
	@mv gestalt-otel ..

clean:
	git clean -fd .
