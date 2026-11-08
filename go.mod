module github.com/mostlygeek/site-llama-swap.ai

go 1.27.3

require (
	github.com/alecthomas/chroma/v2 v2.27.0
	github.com/mostlygeek/llama-swap v0.1.6-0.20261108054722-1249e7c8b53b
	github.com/yuin/goldmark v1.8.7
	github.com/yuin/goldmark-highlighting/v2 v2.0.0-20230729083705-37449abec8cc
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/dlclark/regexp2/v2 v2.2.1 // indirect

tool github.com/mostlygeek/llama-swap/docbot/gensource
