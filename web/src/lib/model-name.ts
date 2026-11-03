// llama-server names a model it was not given an alias for by its file: path, shard suffix and extension.
const FOLDERS = /^.*[\\/]/;
const SHARD = /-\d{5}-of-\d{5}$/;
const EXTENSION = /\.gguf$/i;

/** A model's name for people: "Qwen3.8-Flash-Next-IQ3_S" for /models/qwen/Qwen3.8-Flash-Next-IQ3_S-00001-of-00002.gguf. */
export const modelName = (id: string): string => id.replace(FOLDERS, '').replace(EXTENSION, '').replace(SHARD, '');
