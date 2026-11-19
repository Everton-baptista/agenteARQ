import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import "./style.css";

type Finding = { id: string; subject: string; status: string; message: string };
type Check = {
  id: string;
  subject: string;
  status: string;
  message?: string;
  duration_ms: number;
};
type Task = {
  id: string;
  title: string;
  acceptance: string[];
  checks: string[];
};
type Snapshot = {
  product: {
    name: string;
    goal: string;
    profile: string;
    author: string;
    executor_ids: string[];
  };
  tasks: Task[];
  state: {
    phase: string;
    revision: number;
    completed_tasks: string[];
    verification?: {
      passed: boolean;
      isolation: string;
      finished_at: string;
      checks: Check[];
    };
    review?: { reviewer: string; verdict: string; summary: string };
    release?: { environment: string; head: string; url: string };
    production_check?: { outcome: string; attester: string; summary: string };
  };
  fingerprint: { branch: string; head: string };
  findings: Finding[];
  next: { phase: string; action: string; missing: string[] };
};
type Response = {
  schema_version: string;
  initialized: boolean;
  root: string;
  snapshot?: Snapshot;
  message?: string;
};
const phases = [
  ["draft", "Descoberta"],
  ["specified", "Especificação"],
  ["planned", "Planejamento"],
  ["implementing", "Implementação"],
  ["verifying", "Verificação"],
  ["reviewing", "Revisão"],
  ["pr_ready", "Pronto para PR"],
  ["released", "Liberado"],
  ["production_verified", "Verificado em produção"],
];
const statuses: Record<string, string> = {
  passed: "Aprovado",
  failed: "Reprovado",
  inconclusive: "Inconclusivo",
  stale: "Desatualizado",
  not_run: "Não executado",
  blocked: "Bloqueado",
  cancelled: "Cancelado",
};

const actions: Record<string, string> = {
  draft: "Defina o problema e os critérios de aceite.",
  specified: "Registre a arquitetura e organize as tarefas.",
  planned: "Comece a implementação em um worktree dedicado.",
  implementing: "Conclua as tarefas e reúna as evidências.",
  verifying: "Execute as verificações e resolva as pendências.",
  reviewing: "Solicite uma revisão independente desta versão.",
  pr_ready: "O projeto passou pelo fluxo local. Prepare o pull request e, depois do deploy, registre a liberação.",
  released: "Faça a verificação pós-deploy do release.md e registre o resultado.",
  production_verified: "Em produção e verificado. Acompanhe os sinais do release.md.",
  blocked: "Resolva o impedimento registrado para continuar.",
  cancelled: "O trabalho foi cancelado. Reabra a descoberta para continuar.",
};
function explain(message: string) {
  const known: Record<string, string> = {
    "Check timed out or was cancelled; its container was removed.":
      "A verificação excedeu o tempo disponível ou foi cancelada. O contêiner foi encerrado.",
    "Check returned a nonzero exit code.":
      "A verificação encontrou uma falha. Corrija o problema e execute novamente.",
    "Container or check executable is unavailable; prepare the pinned image with dependencies.":
      "O ambiente de verificação está incompleto. Prepare a imagem com as ferramentas e dependências do projeto.",
    "Configure an installed OCI verifier image pinned by @sha256 digest; local execution is disabled.":
      "Configure uma imagem de verificação instalada e identificada pelo seu hash.",
    "Install Podman or Docker to run isolated checks; no local fallback was used.":
      "Instale Docker ou Podman para executar as verificações em um ambiente isolado.",
    "Source, HEAD, worktree or profile changed; verify and review again.":
      "O código, commit, worktree ou perfil mudou. Execute as verificações e a revisão novamente.",
    "Verification was cancelled or exceeded its time budget.":
      "A verificação foi interrompida ou excedeu o tempo disponível.",
    "Source changed during verification; repeat on a stable worktree.":
      "O código mudou durante a verificação. Execute novamente com uma versão estável.",
  };
  return known[message] ?? message.replace(/^task /, "Tarefa pendente: ");
}

function App() {
  const [data, setData] = useState<Response>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [tab, setTab] = useState("overview");
  const [updated, setUpdated] = useState("");
  async function refresh() {
    setBusy(true);
    try {
      const r = await fetch("/api/status", {
        credentials: "same-origin",
        cache: "no-store",
      });
      if (!r.ok)
        throw new Error(
          r.status === 401
            ? "Sessão local expirada. Abra novamente pelo comando agentarch ui."
            : "Não foi possível consultar o projeto.",
        );
      setData(await r.json());
      setError("");
      setUpdated(
        new Date().toLocaleTimeString("pt-BR", {
          hour: "2-digit",
          minute: "2-digit",
        }),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "Falha de conexão");
    } finally {
      setBusy(false);
    }
  }
  useEffect(() => {
    void refresh();
  }, []);
  const s = data?.snapshot;
  const stage = phases.findIndex((p) => p[0] === s?.state.phase);
  const completed = s?.state.completed_tasks ?? [];
  const findings = s?.findings ?? [];
  const checks = s?.state.verification?.checks ?? [];
  return (
    <div className="shell">
      <a className="skip" href="#main">
        Pular para o conteúdo
      </a>
      <aside className="sidebar">
        <a className="brand" href="/" aria-label="AgenteARQ início">
          <span className="brand-icon">
            A<span>↗</span>
          </span>
          <span>
            Agente<span className="brand-bold">ARQ</span>
            <small>ENGENHARIA DE PRODUTO</small>
          </span>
        </a>
        <div className="workspace-label">SEU WORKSPACE</div>
        <div className="project-name">
          <span className="project-dot" />
          {s?.product.name ?? "Novo projeto"}
        </div>
        <nav aria-label="Navegação principal">
          {[
            ["overview", "◫", "Visão geral"],
            ["tasks", "☷", "Tarefas e critérios"],
            ["quality", "✓", "Qualidade"],
            ["context", "◇", "Contexto"],
          ].map(([id, icon, label]) => (
            <button
              key={id}
              className={tab === id ? "nav-item active" : "nav-item"}
              onClick={() => setTab(id)}
              aria-current={tab === id ? "page" : undefined}
            >
              <span aria-hidden="true">{icon}</span>
              {label}
              {id === "quality" && findings.length > 0 && (
                <b>{findings.length}</b>
              )}
            </button>
          ))}
        </nav>
        <div className="workflow-label">CAMINHO ATÉ A ENTREGA</div>
        <ol className="steps">
          {phases.map(([id, label], i) => (
            <li
              key={id}
              className={stage === i ? "current" : stage > i ? "complete" : ""}
            >
              <span className="step-number">{stage > i ? "✓" : i + 1}</span>
              <span>{label}</span>
              {stage === i && <i>Agora</i>}
            </li>
          ))}
        </ol>
        <div className="local-note">
          <span className="live-dot" /> Ambiente local
          <small>
            O painel lê o projeto. A execução continua no seu assistente.
          </small>
        </div>
      </aside>
      <div className="main-wrap">
        <header className="topbar">
          <span>
            Workspace <span className="slash">/</span>{" "}
            <strong>{s?.product.name ?? "Configuração"}</strong>
          </span>
          <span className="top-status">
            <span className="live-dot" /> LOCAL{" "}
            <span className="avatar">ARQ</span>
          </span>
        </header>
        <main id="main">
          <div className="page-heading">
            <div>
              <p className="eyebrow">CONSTRUA COM CLAREZA</p>
              <h1>
                {tab === "overview"
                  ? "Do plano à entrega."
                  : tab === "tasks"
                    ? "Cada tarefa, um resultado."
                    : tab === "quality"
                      ? "Qualidade que você pode verificar."
                      : "Decisões que acompanham o projeto."}
              </h1>
              <p className="subtitle">
                {s?.product.goal ??
                  "Conecte seu assistente e acompanhe o que falta para transformar sua ideia em um produto."}
              </p>
            </div>
            <button
              className="refresh"
              onClick={() => void refresh()}
              disabled={busy}
            >
              <span aria-hidden="true">↻</span>{" "}
              {busy ? "Atualizando…" : "Atualizar"}
            </button>
          </div>
          {error && (
            <div className="error" role="alert">
              {error}
            </div>
          )}
          {!data && !error && (
            <div className="empty" role="status">
              Lendo o estado do projeto…
            </div>
          )}
          {data && !data.initialized && (
            <section className="onboarding">
              <div className="onboarding-icon">↗</div>
              <p className="eyebrow">UM PONTO DE PARTIDA COMPARTILHADO</p>
              <h2>Seu próximo projeto começa com uma boa pergunta.</h2>
              <p>
                Conte ao seu assistente o que você quer construir. O AgenteARQ
                organiza requisitos, tarefas e evidências para que você saiba o
                que já está pronto e o que ainda precisa de atenção.
              </p>
              <div className="setup-grid">
                <article>
                  <b>01</b>
                  <h3>Defina o produto</h3>
                  <p>
                    Inicialize o workflow pela CLI ou peça ao assistente para
                    fazê-lo.
                  </p>
                  <code>
                    agentarch workflow init --name meu-produto --goal
                    "Objetivo do produto" --author "Seu nome" (sem --profile, ele
                    compara os perfis e pergunta)
                  </code>
                </article>
                <article>
                  <b>02</b>
                  <h3>Conecte seu assistente</h3>
                  <p>
                    Use o plugin para Codex, Claude Code ou Gemini. A instalação
                    preserva suas configurações.
                  </p>
                  <code>agentarch plugin install --help</code>
                </article>
                <article>
                  <b>03</b>
                  <h3>Acompanhe as evidências</h3>
                  <p>
                    Volte aqui para consultar critérios, testes e revisão. Uma
                    pendência sempre explica a próxima ação.
                  </p>
                  <button
                    className="text-button"
                    onClick={() => void refresh()}
                  >
                    Já configurei, atualizar →
                  </button>
                </article>
              </div>
            </section>
          )}
          {s && (
            <>
              <section className="summary-grid" aria-label="Resumo do projeto">
                <article>
                  <span>Fase atual</span>
                  <strong>
                    {phases.find((p) => p[0] === s.state.phase)?.[1] ??
                      statuses[s.state.phase] ??
                      s.state.phase}
                  </strong>
                  <small>Revisão {s.state.revision} do workflow</small>
                </article>
                <article>
                  <span>Tarefas concluídas</span>
                  <strong>
                    {completed.length}
                    <em> / {s.tasks.length}</em>
                  </strong>
                  <small>Vinculadas a critérios de aceite</small>
                </article>
                <article>
                  <span>Verificações aprovadas</span>
                  <strong>
                    {checks.filter((c) => c.status === "passed").length}
                    <em> / {checks.length}</em>
                  </strong>
                  <small>
                    {checks.length
                      ? "Resultado da última execução"
                      : "Ainda não executadas"}
                  </small>
                </article>
                <article className={findings.length ? "attention" : ""}>
                  <span>Pontos de atenção</span>
                  <strong>{findings.length}</strong>
                  <small>
                    {findings.length
                      ? "Precisam de evidência ou correção"
                      : "Nenhum achado registrado"}
                  </small>
                </article>
              </section>
              {tab === "overview" && (
                <>
                  <section className="next-card">
                    <span className="next-icon">↗</span>
                    <div>
                      <p className="eyebrow">SEU PRÓXIMO PASSO</p>
                      <h2>{actions[s.next.phase] ?? s.next.action}</h2>
                      {(s.next.missing ?? []).length > 0 && (
                        <ul>
                          {s.next.missing.map((m, i) => (
                            <li key={i}>{explain(m)}</li>
                          ))}
                        </ul>
                      )}
                    </div>
                    <span className="pill">
                      {phases.find((p) => p[0] === s.next.phase)?.[1] ??
                        s.next.phase}
                    </span>
                  </section>
                  <div className="two-columns">
                    <section className="panel">
                      <div className="section-head">
                        <h2>Precisa de atenção</h2>
                        <button
                          className="text-button"
                          onClick={() => setTab("quality")}
                        >
                          Ver qualidade →
                        </button>
                      </div>
                      <Findings items={findings.slice(0, 5)} />
                    </section>
                    <section className="panel">
                      <div className="section-head">
                        <h2>Em construção</h2>
                        <span className="muted">{s.tasks.length} tarefas</span>
                      </div>
                      <Tasks
                        tasks={s.tasks.slice(0, 4)}
                        completed={completed}
                      />
                    </section>
                  </div>
                </>
              )}
              {tab === "tasks" && (
                <section className="panel">
                  <div className="section-head">
                    <h2>Tarefas e critérios de aceite</h2>
                    <span className="pill">{s.product.profile}</span>
                  </div>
                  <Tasks tasks={s.tasks} completed={completed} detailed />
                </section>
              )}
              {tab === "quality" && (
                <>
                  <section className="panel">
                    <div className="section-head">
                      <h2>Achados e pendências</h2>
                      <span className="muted">
                        Ausência de evidência não é aprovação
                      </span>
                    </div>
                    <Findings items={findings} />
                  </section>
                  <section className="panel">
                    <h2>Última verificação</h2>
                    {checks.length ? (
                      <div className="check-list">
                        {checks.map((c, i) => (
                          <div className="check-row" key={c.subject + c.id + i}>
                            <div>
                              <b>{c.id}</b>
                              <p>{explain(c.message ?? c.subject)}</p>
                            </div>
                            <span className={"badge " + c.status}>
                              {statuses[c.status] ?? c.status}
                            </span>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <div className="empty">
                        As verificações ainda não foram executadas. Use a
                        ferramenta de verificação no assistente ou a CLI.
                      </div>
                    )}
                    {s.state.verification && (
                      <p className="muted">
                        Isolamento: {s.state.verification.isolation} ·{" "}
                        {new Date(
                          s.state.verification.finished_at,
                        ).toLocaleString("pt-BR")}
                      </p>
                    )}
                  </section>
                </>
              )}
              {tab === "context" && (
                <div className="two-columns">
                  <section className="panel">
                    <h2>Contrato do produto</h2>
                    <dl>
                      <dt>Objetivo</dt>
                      <dd>{s.product.goal}</dd>
                      <dt>Responsável</dt>
                      <dd>{s.product.author}</dd>
                      <dt>Perfil de engenharia</dt>
                      <dd>{s.product.profile}</dd>
                      <dt>Executores declarados</dt>
                      <dd>
                        {s.product.executor_ids?.join(", ") ||
                          "Nenhum declarado"}
                      </dd>
                      <dt>Branch</dt>
                      <dd>{s.fingerprint.branch || "Sem branch"}</dd>
                      <dt>Commit</dt>
                      <dd>
                        <code>{s.fingerprint.head || "Ainda sem commit"}</code>
                      </dd>
                    </dl>
                  </section>
                  <section className="panel">
                    <h2>Revisão independente</h2>
                    {s.state.review ? (
                      <>
                        <p>
                          <b>{s.state.review.reviewer}</b> ·{" "}
                          {s.state.review.verdict === "approved"
                            ? "Aprovado"
                            : s.state.review.verdict === "changes_requested"
                              ? "Ajustes solicitados"
                              : s.state.review.verdict}
                        </p>
                        <p>{s.state.review.summary}</p>
                      </>
                    ) : (
                      <div className="empty">
                        Nenhuma revisão registrada. A conclusão exige revisão
                        separada da autoria e evidências atuais.
                      </div>
                    )}
                    <div className="context-tip">
                      <b>Seu contexto continua com você.</b>
                      <p>
                        Requisitos e tarefas ficam no repositório. Você pode
                        continuar em outro assistente sem reconstruir todas as
                        decisões.
                      </p>
                    </div>
                  </section>
                </div>
              )}
            </>
          )}
          <footer>
            <span>AgenteARQ · Evidência antes de conclusão.</span>
            <span aria-live="polite">
              {updated ? "Atualizado às " + updated : "Painel local"} · Somente
              leitura
            </span>
          </footer>
        </main>
      </div>
    </div>
  );
}
function Findings({ items }: { items: Finding[] }) {
  return items.length ? (
    <ul className="findings">
      {items.map((f, i) => (
        <li key={f.id + f.subject + i}>
          <span className={"finding-dot " + f.status} />
          <div>
            <div className="finding-title">
              <b>{f.id}</b>
              <span className={"badge " + f.status}>
                {statuses[f.status] ?? f.status}
              </span>
            </div>
            <p>{explain(f.message)}</p>
            <small>{f.subject}</small>
          </div>
        </li>
      ))}
    </ul>
  ) : (
    <div className="empty">
      Nenhum achado registrado. Consulte as verificações para confirmar o que
      foi avaliado.
    </div>
  );
}
function Tasks({
  tasks,
  completed,
  detailed = false,
}: {
  tasks: Task[];
  completed: string[];
  detailed?: boolean;
}) {
  return tasks.length ? (
    <ul className="tasks">
      {tasks.map((t) => (
        <li key={t.id}>
          <span
            className={completed.includes(t.id) ? "task-box done" : "task-box"}
          >
            {completed.includes(t.id) ? "✓" : ""}
          </span>
          <div>
            <small>{t.id}</small>
            <h3>{t.title}</h3>
            {detailed && (
              <>
                <ul className="acceptance">
                  {t.acceptance.map((a, i) => (
                    <li key={i}>{a}</li>
                  ))}
                </ul>
                <p className="muted">
                  Verificações: {t.checks.join(", ") || "Ainda não vinculadas"}
                </p>
              </>
            )}
          </div>
        </li>
      ))}
    </ul>
  ) : (
    <div className="empty">O plano ainda não possui tarefas.</div>
  );
}
createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
