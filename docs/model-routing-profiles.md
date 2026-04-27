# Model Routing Profiles

Updated: 2026-04-27

Auto routing still starts from the user's configured model scores. The profile layer only adds a small task-specific bonus when a configured model name matches a known benchmark profile. This keeps user scoring and rate limits authoritative while improving ties and close decisions.

## Sources Used

- GPT-5.5: OpenAI describes it as a strong agentic coding model and publishes coding, professional, academic, long-context, and abstract-reasoning evals. Source: https://openai.com/index/introducing-gpt-5-5/
- Kimi K2.6: Moonshot positions it around coding, long-horizon execution, and agent swarm workflows. Source: https://www.kimi.com/ai-models/kimi-k2-6
- GLM-5.1: Z.AI describes balanced benchmark coverage across reasoning, coding, agents, tool use, browsing, and long-horizon engineering. Source: https://docs.z.ai/guides/llm/glm-5.1
- MiniMax M2.7: MiniMax publishes software-engineering and agent/tool results, and documents an M2.7-highspeed variant with identical results and faster speed. Source: https://www.minimax.io/models/text/m27
- DeepSeek V4 Pro: DeepSeek's model card reports 1M context support plus coding, reasoning, and agentic benchmark rows for V4-Pro modes. Source: https://huggingface.co/deepseek-ai/DeepSeek-V4-Pro
- Gemma 4 26B A4B: A recent arXiv benchmark covers Gemma-4-26B-A4B in an accuracy/latency/VRAM tradeoff comparison; the router treats this as a lower-cost local/private option rather than a top agentic coding default. Source: https://arxiv.org/abs/2604.07035
- cc-switch: Used for multi-agent config targets, file formats, and safe-write behavior. Source: https://github.com/farion1231/cc-switch

## Routing Intent

- Agentic coding and CLI/tool-use tasks should prefer Kimi K2.6, GPT-5.5, GLM-5.1, or DeepSeek V4 Pro when their user scores are otherwise close.
- Debugging and architecture tasks should favor reasoning plus coding rather than pure speed.
- Simple translation, rewriting, and low-risk prompts should favor high speed and cost efficiency, especially MiniMax M2.7-highspeed or local/private Gemma profiles.
- Creative writing should use creativity-heavy weighting but still respect user score and current capacity.
- All model-specific bonuses are small enough that a user's explicit score, inactive state, RPM/TPM capacity, or manual mode still wins.
