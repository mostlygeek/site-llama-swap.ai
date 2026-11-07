package server

// AgentConfig is what the Help page needs to run the agent in the browser. It
// is embedded in the page as JSON.
type AgentConfig struct {
	// SystemPrompt is llama-swap's Docs Agent prompt, minus the lines for
	// tools this site does not serve (see docbot.Handler.SystemPrompt).
	SystemPrompt string `json:"systemPrompt"`

	// Model is the Hugging Face repository holding the ONNX weights, and
	// Revision the commit to load, so a new upload cannot change the site
	// without a deploy.
	Model    string `json:"model"`
	Revision string `json:"revision"`

	// TransformersURL is the Transformers.js build the worker imports.
	TransformersURL string `json:"transformersUrl"`

	// MaxNewTokens bounds one model turn, reasoning included.
	MaxNewTokens int `json:"maxNewTokens"`

	// MaxIterations bounds model turns in one answer, so a model that keeps
	// calling tools stops eventually.
	MaxIterations int `json:"maxIterations"`

	Suggestions []string `json:"suggestions"`

	// DocLinks maps the document ids the agent names in its answers to their
	// pages on the site, so the Help page can link them.
	DocLinks map[string]string `json:"docLinks"`
}

// Defaults for the model. The model card tests Transformers.js 4.3.2 only.
const (
	DefaultModel           = "llama-swap/LFM3-1.7B-HelpAgent"
	DefaultModelRevision   = "0898fda4ce6303c23db0e93ea2288ab7c4ab867c"
	DefaultTransformersURL = "https://cdn.jsdelivr.net/npm/@huggingface/transformers@4.3.2/dist/transformers.min.js"
)

// suggestions are the questions offered on an empty Help page. Each must be
// answerable from llama-swap's knowledge base; the comment names the guide.
// Unlike llama-swap's own Help page there is no running config here, so
// questions about "this server" are left out.
var suggestions = []string{
	// guides/model-runtime/ttl-and-unloading
	"How do I unload a model after five minutes idle?",
	// guides/routing/groups-and-matrix
	"How can I keep several models loaded at once?",
	// guides/configuration/macros
	"How do I avoid repeating a long command for every model?",
	// guides/connectivity/remote-peers
	"How do I use models on another computer?",
	// guides/model-runtime/troubleshooting-model-wont-load
	"A model won't start. How do I fix it?",
	// guides/api-integration/api-keys-and-auth
	"How do I require an API key?",
}
