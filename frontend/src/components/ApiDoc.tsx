import { BookOpen } from "lucide-react";

/**
 * OpenAI API 接口与调用说明（统一网关）。
 * 展示在 API Keys 页面，帮助用户理解支持哪些端点、如何鉴权与调用。
 */
export default function ApiDoc() {
  const base = "{portal}/api/v1/gateway/{实例slug}/openapi/v1";

  return (
    <details className="mb-5 rounded-xl border border-zinc-800 bg-zinc-900/60">
      <summary className="flex cursor-pointer items-center gap-2 px-4 py-3 text-sm font-medium text-zinc-300 hover:text-amber-300">
        <BookOpen className="h-4 w-4 text-amber-400" />
        OpenAI API 接口与调用说明
      </summary>

      <div className="space-y-5 border-t border-zinc-800 px-5 py-4 text-sm">
        {/* 接入说明 */}
        <section>
          <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-zinc-500">接入方式</h3>
          <ol className="list-decimal space-y-1 pl-5 text-zinc-400">
            <li>在本页面创建 API Key（<code className="text-amber-300">hp_…</code>，明文只显示一次）</li>
            <li>通过统一网关调用：<code className="rounded bg-zinc-950 px-1.5 py-0.5 text-xs text-sky-300">{base}/…</code></li>
            <li>请求头携带 <code className="rounded bg-zinc-950 px-1.5 py-0.5 text-xs text-emerald-300">X-API-Key: hp_…</code>（或 <code className="text-emerald-300">Authorization: Bearer hp_…</code>）</li>
            <li>实例 slug 在「实例管理」列表与详情页可见；每个实例对应独立的统一网关地址</li>
          </ol>
        </section>

        {/* 端点表 */}
        <section>
          <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-zinc-500">支持的接口</h3>
          <div className="overflow-hidden rounded-lg border border-zinc-800">
            <table className="w-full text-xs">
              <thead className="bg-zinc-950/70 text-left text-zinc-500">
                <tr>
                  <th className="px-3 py-2">方法</th>
                  <th className="px-3 py-2">路径</th>
                  <th className="px-3 py-2">说明</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-800/70">
                <tr className="bg-zinc-950/30">
                  <td colSpan={3} className="px-3 py-1.5 font-medium text-zinc-400">OpenAI 兼容</td>
                </tr>
                <tr>
                  <td className="px-3 py-2 font-mono text-emerald-300">GET</td>
                  <td className="px-3 py-2 font-mono text-sky-300">/v1/models</td>
                  <td className="px-3 py-2 text-zinc-400">模型列表（配置探测）</td>
                </tr>
                <tr>
                  <td className="px-3 py-2 font-mono text-emerald-300">POST</td>
                  <td className="px-3 py-2 font-mono text-sky-300">/v1/chat/completions</td>
                  <td className="px-3 py-2 text-zinc-400">对话补全（支持 <code className="text-zinc-300">stream: true</code> SSE 流式）</td>
                </tr>
                <tr>
                  <td className="px-3 py-2 font-mono text-emerald-300">POST</td>
                  <td className="px-3 py-2 font-mono text-sky-300">/v1/responses</td>
                  <td className="px-3 py-2 text-zinc-400">OpenAI Responses API</td>
                </tr>
                <tr>
                  <td className="px-3 py-2 font-mono text-emerald-300">GET / DELETE</td>
                  <td className="px-3 py-2 font-mono text-sky-300">/v1/responses/{`{id}`}</td>
                  <td className="px-3 py-2 text-zinc-400">查询 / 删除响应</td>
                </tr>
                <tr className="bg-zinc-950/30">
                  <td colSpan={3} className="px-3 py-1.5 font-medium text-zinc-400">Hermes 扩展</td>
                </tr>
                <tr>
                  <td className="px-3 py-2 font-mono text-emerald-300">POST</td>
                  <td className="px-3 py-2 font-mono text-sky-300">/v1/runs</td>
                  <td className="px-3 py-2 text-zinc-400">启动运行（SSE 事件流，携带完整 agent 工具能力）</td>
                </tr>
                <tr>
                  <td className="px-3 py-2 font-mono text-emerald-300">GET</td>
                  <td className="px-3 py-2 font-mono text-sky-300">/v1/runs/{`{run_id}`}</td>
                  <td className="px-3 py-2 text-zinc-400">运行状态查询</td>
                </tr>
                <tr>
                  <td className="px-3 py-2 font-mono text-emerald-300">GET</td>
                  <td className="px-3 py-2 font-mono text-sky-300">/v1/runs/{`{run_id}`}/events</td>
                  <td className="px-3 py-2 text-zinc-400">运行事件流（SSE）</td>
                </tr>
                <tr>
                  <td className="px-3 py-2 font-mono text-emerald-300">POST</td>
                  <td className="px-3 py-2 font-mono text-sky-300">/v1/runs/{`{run_id}`}/stop</td>
                  <td className="px-3 py-2 text-zinc-400">停止运行</td>
                </tr>
                <tr>
                  <td className="px-3 py-2 font-mono text-emerald-300">GET / POST</td>
                  <td className="px-3 py-2 font-mono text-sky-300">/api/sessions</td>
                  <td className="px-3 py-2 text-zinc-400">会话列表 / 创建会话</td>
                </tr>
                <tr>
                  <td className="px-3 py-2 font-mono text-emerald-300">POST</td>
                  <td className="px-3 py-2 font-mono text-sky-300">/api/sessions/{`{id}`}/chat</td>
                  <td className="px-3 py-2 text-zinc-400">在会话内对话（延续上下文）</td>
                </tr>
                <tr>
                  <td className="px-3 py-2 font-mono text-emerald-300">GET</td>
                  <td className="px-3 py-2 font-mono text-sky-300">/v1/capabilities · /v1/skills · /v1/toolsets</td>
                  <td className="px-3 py-2 text-zinc-400">Agent 能力/技能/工具集探测</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        {/* 调用示例 */}
        <section>
          <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-zinc-500">调用示例</h3>
          <div className="space-y-3">
            <CodeBlock
              title="① 模型列表"
              code={`curl ${base}/models \\\n  -H "X-API-Key: hp_你的Key"`}
            />
            <CodeBlock
              title="② 对话补全（普通）"
              code={`curl ${base}/chat/completions \\\n  -H "X-API-Key: hp_你的Key" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"你好"}]}'`}
            />
            <CodeBlock
              title="③ 对话补全（流式 SSE）"
              code={`curl -N ${base}/chat/completions \\\n  -H "X-API-Key: hp_你的Key" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"写一首诗"}],"stream":true}'`}
            />
            <CodeBlock
              title="④ Python（OpenAI SDK）"
              code={`from openai import OpenAI\n\nclient = OpenAI(\n    base_url="${base}",\n    api_key="hp_你的Key",\n)\n\nresp = client.chat.completions.create(\n    model="gpt-4o",\n    messages=[{"role": "user", "content": "你好"}],\n)\nprint(resp.choices[0].message.content)`}
            />
          </div>
        </section>

        <p className="text-xs text-zinc-600">
          提示：`{`model`}` 字段填写实例已配置的模型名（可在「实例编辑 → 默认模型参数」查看）；未配置模型时可用 hermes 默认模型名。
        </p>
      </div>
    </details>
  );
}

function CodeBlock({ title, code }: { title: string; code: string }) {
  return (
    <div className="overflow-hidden rounded-lg border border-zinc-800">
      <div className="border-b border-zinc-800 bg-zinc-950/70 px-3 py-1.5 text-xs font-medium text-zinc-400">{title}</div>
      <pre className="overflow-x-auto bg-zinc-950 p-3 font-mono text-[11px] leading-relaxed text-zinc-300">{code}</pre>
    </div>
  );
}
