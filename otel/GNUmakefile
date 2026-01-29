OCB_VER := 0.144.0

Linux_x86_64:
	$(info Open Telemetry Build)
	@[ -r ocb ] || curl --proto '=https' --tlsv1.2 -fL -o ocb https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/cmd%2Fbuilder%2Fv${OCB_VER}/ocb_${OCB_VER}_linux_amd64
	@./ocb --config builder-config.yaml
	@cp gestalt-otel ..

clean:
	git clean -fd .
