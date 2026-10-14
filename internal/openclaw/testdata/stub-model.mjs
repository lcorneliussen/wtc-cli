#!/usr/bin/env node
// A scripted stand-in for a model, speaking OpenAI chat completions. It lets a
// test run real agent turns on a gateway without a model account: each model
// request is answered with the next step of a script.
//
//   STUB_PORT=18999 STUB_STEPS=/tmp/stub/steps.json STUB_LOG=/tmp/stub/requests.jsonl node stub-model.mjs
//
// Steps file: {"steps":[ ... ]}, each step one of
//   {"tool": name, "arguments": object}   one tool call
//   {"carryOut": n}                       the tool call that intent n of the latest
//                                         wtc plan among the tool results asks for
//   {"text": string}                      the final assistant text
// with optional
//   "match": regex       only for a request whose messages match
//   "notMatch": regex    only for a request whose messages do not match
//   "cwd": path          only for a turn whose working directory is path, which
//                        tells a session's turns from those of its parent or children
//   "noTools": true      also for a request that offers no tools
// A request takes the first step that is unused and eligible; when none is, it
// gets STUB_DEFAULT_TEXT. The script restarts when the steps file changes.
// Every request is appended to STUB_LOG as one JSON line.
import http from "node:http";
import fs from "node:fs";

const PORT = Number(process.env.STUB_PORT || 18999);
const STEPS = process.env.STUB_STEPS || "/tmp/stub/steps.json";
const LOG = process.env.STUB_LOG || "/tmp/stub/requests.jsonl";
const DEFAULT_TEXT = process.env.STUB_DEFAULT_TEXT || "stub: no scripted step left";
const MODEL = process.env.STUB_MODEL || "stub-model";

let seq = 0;
let stepsMtime = -1;
let steps = [];
let consumed = [];

function loadSteps() {
  let st;
  try { st = fs.statSync(STEPS); } catch { steps = []; consumed = []; stepsMtime = -1; return; }
  if (st.mtimeMs === stepsMtime) return;
  stepsMtime = st.mtimeMs;
  const parsed = JSON.parse(fs.readFileSync(STEPS, "utf8"));
  steps = Array.isArray(parsed) ? parsed : parsed.steps || [];
  consumed = steps.map(() => false);
}

function pickStep(body) {
  loadSteps();
  const hasTools = Array.isArray(body.tools) && body.tools.length > 0;
  const hay = JSON.stringify(body.messages ?? body.input ?? "");
  for (let i = 0; i < steps.length; i++) {
    if (consumed[i]) continue;
    const s = steps[i];
    if (!hasTools && !s.noTools) continue;
    if (s.match && !new RegExp(s.match).test(hay)) continue;
    if (s.notMatch && new RegExp(s.notMatch).test(hay)) continue;
    if (s.cwd && !hay.includes(`Working directory: ${s.cwd}\\n`)) continue;
    consumed[i] = true;
    return { index: i, step: s.carryOut === undefined ? s : carryOut(body, s.carryOut) };
  }
  return { index: -1, step: { text: DEFAULT_TEXT } };
}

// The first JSON object in text, which may be followed by other output.
function firstObject(text) {
  for (let start = text.indexOf("{"); start >= 0; start = text.indexOf("{", start + 1)) {
    let depth = 0, inString = false;
    for (let i = start; i < text.length; i++) {
      const ch = text[i];
      if (inString) { if (ch === "\\") i++; else if (ch === '"') inString = false; continue; }
      if (ch === '"') inString = true;
      else if (ch === "{") depth++;
      else if (ch === "}" && --depth === 0) {
        try { return JSON.parse(text.slice(start, i + 1)); } catch { break; }
      }
    }
  }
  return null;
}

// What an agent does with a wtc plan: call the tool an intent names with the
// intent's args, unchanged.
function carryOut(body, n) {
  const results = (body.messages ?? []).filter((m) => m.role === "tool");
  // The plan is in the latest tool result that holds one: carrying out its
  // first intent puts that tool's own result after it.
  for (let i = results.length - 1; i >= 0; i--) {
    const content = results[i].content;
    const text = typeof content === "string" ? content : JSON.stringify(content ?? "");
    // A tool result may arrive as a JSON string of the output.
    const plan = firstObject(text) ?? firstObject(text.replace(/\\n/g, "\n").replace(/\\"/g, '"'));
    if (!Array.isArray(plan?.intents)) continue;
    const intent = plan.intents[n];
    if (!intent?.tool) return { text: `stub: intent ${n} of the plan names no tool` };
    return { tool: intent.tool, arguments: intent.args };
  }
  return { text: "stub: no wtc plan in any tool result" };
}

function message(step, id) {
  if (step.tool) {
    return {
      role: "assistant", content: null,
      tool_calls: [{ index: 0, id: `call_stub_${id}`, type: "function",
        function: { name: step.tool, arguments: JSON.stringify(step.arguments ?? {}) } }],
    };
  }
  return { role: "assistant", content: String(step.text ?? "") };
}

function respond(res, body, step, id) {
  const created = Math.floor(Date.now() / 1000);
  const finish = step.tool ? "tool_calls" : "stop";
  const usage = { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 };
  const base = { id: `chatcmpl-stub-${id}`, created, model: body.model || MODEL };
  const msg = message(step, id);
  if (!body.stream) {
    res.writeHead(200, { "content-type": "application/json" });
    res.end(JSON.stringify({ ...base, object: "chat.completion",
      choices: [{ index: 0, message: msg, finish_reason: finish }], usage }));
    return;
  }
  res.writeHead(200, { "content-type": "text/event-stream", "cache-control": "no-cache", connection: "keep-alive" });
  const send = (o) => res.write(`data: ${JSON.stringify({ ...base, object: "chat.completion.chunk", ...o })}\n\n`);
  send({ choices: [{ index: 0, delta: { role: "assistant", content: "" }, finish_reason: null }] });
  const delta = step.tool ? { tool_calls: msg.tool_calls } : { content: msg.content };
  send({ choices: [{ index: 0, delta, finish_reason: null }] });
  send({ choices: [{ index: 0, delta: {}, finish_reason: finish }] });
  send({ choices: [], usage });
  res.end("data: [DONE]\n\n");
}

http.createServer((req, res) => {
  const chunks = [];
  req.on("data", (c) => chunks.push(c));
  req.on("end", () => {
    const raw = Buffer.concat(chunks).toString("utf8");
    let body = {};
    try { body = raw ? JSON.parse(raw) : {}; } catch { body = { _raw: raw }; }
    const id = ++seq;
    const entry = { seq: id, ts: new Date().toISOString(), method: req.method, url: req.url };
    if (req.method === "POST" && req.url.startsWith("/__reset")) {
      stepsMtime = -1; loadSteps();
      res.writeHead(200, { "content-type": "application/json" });
      res.end(JSON.stringify({ ok: true, steps: steps.length }));
    } else if (req.method === "GET" && req.url.includes("/models")) {
      res.writeHead(200, { "content-type": "application/json" });
      res.end(JSON.stringify({ object: "list", data: [{ id: MODEL, object: "model", owned_by: "stub" }] }));
    } else if (req.method === "POST" && req.url.includes("/chat/completions")) {
      const { index, step } = pickStep(body);
      entry.served = { index, step };
      respond(res, body, step, id);
    } else {
      res.writeHead(404, { "content-type": "application/json" });
      res.end(JSON.stringify({ error: { message: `stub: unhandled ${req.method} ${req.url}` } }));
    }
    entry.body = body;
    fs.appendFileSync(LOG, JSON.stringify(entry) + "\n");
  });
}).listen(PORT, "127.0.0.1", () => console.log(`stub model listening on 127.0.0.1:${PORT}`));
