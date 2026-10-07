// Run in a temporary Node project with @huggingface/transformers@4.3.1.
// The mounted model directory contains only the pinned text assets.
import { AutoConfig, AutoModel, AutoTokenizer, env } from '@huggingface/transformers';
import { writeFile } from 'node:fs/promises';

env.allowRemoteModels = false;
const directory = process.argv[2];
const destination = process.argv[3];
const configuration = await AutoConfig.from_pretrained(directory);
configuration.vision_config = null;
configuration.audio_config = null;
const model = await AutoModel.from_pretrained(directory, {
  config: configuration, device: 'cpu', dtype: 'q8', local_files_only: true,
});
const tokenizer = await AutoTokenizer.from_pretrained(directory, { local_files_only: true });
const inputs = [
  'task: search result | query: Which planet is known as the Red Planet?',
  'title: none | text: Mars, known for its reddish appearance, is often referred to as the Red Planet.',
  'title: none | text: PostgreSQL stores workspace memories.',
  'title: none | text: My name is Matheus.',
  'task: search result | query: What is the user called?',
];
const records = [];
for (const text of inputs) {
  const tokens = await tokenizer([text]);
  const output = await model(tokens);
  records.push({ text, tokens: Array.from(tokens.input_ids.data, Number), embedding: Array.from(output.sentence_embedding.data) });
}
await writeFile(destination, JSON.stringify(records));
await model.dispose();
