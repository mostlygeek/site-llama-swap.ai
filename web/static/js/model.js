// Loading and running LFM3-1.7B-HelpAgent with Transformers.js.
//
// This follows the model card's "Run it in the browser" section. The
// Transformers.js module is passed in rather than imported, so the same code
// runs in the browser worker (WebGPU) and under Node for testing (CPU).

/**
 * Loads the tokenizer and model.
 *
 * @param {object} T the Transformers.js module
 * @param {object} opts
 * @param {string} opts.model Hugging Face repository id
 * @param {string} opts.revision commit or branch
 * @param {string} opts.device "webgpu" in the browser
 * @param {string} opts.dtype "q4f16" or "q4"
 * @param {(info: object) => void} [opts.onProgress] Transformers.js progress events
 */
export async function loadModel(T, { model, revision, device, dtype, onProgress }) {
  const options = { revision, progress_callback: onProgress };
  const tokenizer = await T.AutoTokenizer.from_pretrained(model, options);
  const lm = await T.AutoModelForCausalLM.from_pretrained(model, { ...options, dtype, device });
  return { tokenizer, model: lm };
}

/**
 * Reads a prompt into a new cache in chunks, leaving the last token for
 * generate(). One pass over a long Docs Agent prompt runs the GPU out of
 * memory or, with q4f16, gives wrong output, so the model card prefills 256
 * tokens at a time.
 */
async function prefill(T, model, ids, { chunkSize = 256, shouldStop } = {}) {
  const cache = new T.DynamicCache();
  const total = ids.dims[1];
  try {
    for (let start = 0; start < total - 1; start += chunkSize) {
      if (shouldStop?.()) {
        const error = new Error("Generation stopped");
        error.name = "AbortError";
        throw error;
      }
      const end = Math.min(start + chunkSize, total - 1);
      const output = await model({
        input_ids: ids.slice(null, [start, end]),
        attention_mask: T.ones([1, end]),
        past_key_values: cache,
        num_logits_to_keep: new T.Tensor("int64", [1n], []),
      });
      output.logits.dispose();
      cache.update(
        Object.fromEntries(
          Object.entries(output)
            .filter(([name]) => name.startsWith("present"))
            .map(([name, tensor]) => [
              name.replace("present_conv", "past_conv").replace("present", "past_key_values"),
              tensor,
            ]),
        ),
      );
    }
  } catch (error) {
    await cache.dispose();
    throw error;
  }
  return cache;
}

/**
 * Runs one model turn.
 *
 * @param {object} T the Transformers.js module
 * @param {{tokenizer: object, model: object}} loaded from loadModel()
 * @param {object} opts
 * @param {object[]} opts.messages chat messages, system prompt first
 * @param {object[]} opts.tools OpenAI-shaped tool definitions
 * @param {number} opts.maxNewTokens
 * @param {(text: string) => void} [opts.onText] streamed text, special tokens included
 * @param {object} [opts.stoppingCriteria] an InterruptableStoppingCriteria
 * @param {() => boolean} [opts.shouldStop] checked between prefill chunks
 */
export async function generateTurn(T, { tokenizer, model }, { messages, tools, maxNewTokens, onText, stoppingCriteria, shouldStop }) {
  const prompt = tokenizer.apply_chat_template(messages, {
    tools,
    add_generation_prompt: true,
    tokenize: false,
  });
  const inputs = tokenizer(prompt, { add_special_tokens: false, return_tensors: "pt" });
  const promptTokens = inputs.input_ids.dims[1];

  const t0 = performance.now();
  const cache = await prefill(T, model, inputs.input_ids, { shouldStop });
  const t1 = performance.now();

  const streamer = new T.TextStreamer(tokenizer, {
    skip_prompt: true,
    skip_special_tokens: false,
    callback_function: (text) => onText?.(text),
  });

  let output;
  try {
    output = await model.generate({
      ...inputs,
      past_key_values: cache,
      max_new_tokens: maxNewTokens,
      streamer,
      ...(stoppingCriteria ? { stopping_criteria: stoppingCriteria } : {}),
    });
  } finally {
    await cache.dispose();
  }
  const t2 = performance.now();

  const generated = output.slice(null, [promptTokens, null]);
  const text = tokenizer.batch_decode(generated, { skip_special_tokens: false })[0];

  return {
    text,
    promptTokens,
    completionTokens: generated.dims[1],
    prefillMs: t1 - t0,
    generateMs: t2 - t1,
  };
}
