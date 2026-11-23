package main

// Plain-language explanations for workflow refusals.
//
// A gate refusal is correct and terse: "cannot skip workflow phases". To the engineer who wrote
// it, that is the whole story. To a product owner who has never seen a phase diagram it is a
// wall: something said no, and nothing says what to do next. Every refusal the workflow can
// give therefore has an entry here with an id, what it means in ordinary words and the next
// step, in each language the interview speaks. The original message is always printed too —
// the explanation adds, it never replaces, so nothing precise is lost in translation.
//
// TestEveryWorkflowRefusalIsExplained reads the workflow's source and fails when a refusal
// message has no entry, so a new gate cannot ship speaking only engineer.

import (
	"fmt"
	"io"
	"regexp"
)

type refusal struct {
	ID      string
	Pattern *regexp.Regexp
	Text    map[string][2]string // language → {what it means, what to do next}
}

func r(id, pattern, enMeaning, enNext, ptMeaning, ptNext string) refusal {
	return refusal{ID: id, Pattern: regexp.MustCompile(pattern), Text: map[string][2]string{
		"en": {enMeaning, enNext}, "pt-BR": {ptMeaning, ptNext},
	}}
}

var refusals = []refusal{
	r("workflow.already_initialized", `^workflow already initialized`,
		"This product's workflow already exists.", "Run `agentarch workflow status` to see where it stands.",
		"O workflow deste produto já existe.", "Rode `agentarch workflow status` para ver onde ele está."),
	r("workflow.adopt_mismatch", `^product\.yaml already (names this product|uses profile)`,
		"product.yaml already records a different name or profile than the one given.", "Run the command again without --name or --profile to adopt what is recorded.",
		"O product.yaml já registra um nome ou perfil diferente do informado.", "Rode o comando de novo sem --name ou --profile para adotar o que está registrado."),
	r("workflow.name_required", `^product name is required`,
		"The product needs a name.", "Give it one with --name, for example --name my-shop.",
		"O produto precisa de um nome.", "Informe com --name, por exemplo --name minha-loja."),
	r("workflow.profile_required", `^choose an engineering profile`,
		"Nobody has chosen how this product will be checked yet; AgenteARQ never chooses for you.", "Read the profiles listed and pick the one that matches your technology, or universal for any other.",
		"Ninguém escolheu ainda como este produto será verificado; o AgenteARQ nunca escolhe por você.", "Leia os perfis listados e escolha o que combina com a sua tecnologia, ou universal para qualquer outra."),
	r("workflow.unknown_profile", `^unknown engineering profile`,
		"That profile name does not exist.", "Run `agentarch workflow init` on a terminal to see and compare the profiles.",
		"Esse nome de perfil não existe.", "Rode `agentarch workflow init` num terminal para ver e comparar os perfis."),
	r("workflow.refuse_overwrite", `^refusing to overwrite`,
		"A file the workflow would create already exists, and AgenteARQ never overwrites your work.", "Move or rename that file, or adopt the existing product instead.",
		"Um arquivo que o workflow criaria já existe, e o AgenteARQ nunca sobrescreve o seu trabalho.", "Mova ou renomeie esse arquivo, ou adote o produto existente."),
	r("workflow.task_invalid", `^(invalid task id|duplicate task id|task dependency cycle|unknown task dependency)`,
		"tasks.yaml has a task with a bad or repeated id, or tasks that depend on each other in a circle or on a task that does not exist.", "Ask your assistant to fix tasks.yaml so every task has a unique id and depends only on earlier tasks.",
		"O tasks.yaml tem uma tarefa com id inválido ou repetido, ou tarefas que dependem umas das outras em círculo ou de uma tarefa que não existe.", "Peça ao seu assistente para corrigir o tasks.yaml: cada tarefa com id único e dependendo só de tarefas anteriores."),
	r("workflow.note_too_long", `^checkpoint note exceeds`,
		"The checkpoint note is too long.", "Keep the note to a short summary; details belong in the repository.",
		"A nota do checkpoint é longa demais.", "Deixe a nota como um resumo curto; os detalhes ficam no repositório."),
	r("workflow.gate_owned_phase", `^only (successful verification and independent review|record-release and record-production-check) can advance`,
		"That step can only be reached by the evidence itself, never by declaring it.", "Run `agentarch workflow verify`, ask for an independent review, or record the release, depending on the step.",
		"Essa etapa só é alcançada pela própria evidência, nunca declarando que ela aconteceu.", "Rode `agentarch workflow verify`, peça uma revisão independente ou registre o release, conforme a etapa."),
	r("workflow.unknown_phase", `^unknown workflow phase`,
		"That is not one of the workflow's steps.", "Run `agentarch workflow status` to see the current step and the next one.",
		"Isso não é uma das etapas do workflow.", "Rode `agentarch workflow status` para ver a etapa atual e a próxima."),
	r("workflow.reason_required", `^blocked/cancelled requires a reason`,
		"Pausing or cancelling the work needs a reason.", "Add --note with one sentence saying why.",
		"Pausar ou cancelar o trabalho exige um motivo.", "Adicione --note com uma frase dizendo por quê."),
	r("workflow.reopen_at_draft", `^cancelled work must reopen at draft`,
		"Cancelled work starts over from the first step.", "Checkpoint draft to reopen it.",
		"Trabalho cancelado recomeça da primeira etapa.", "Faça o checkpoint draft para reabrir."),
	r("workflow.cannot_skip", `^cannot skip workflow phases`,
		"Steps happen in order, so nothing important is skipped.", "Run `agentarch workflow status` and complete the next step first.",
		"As etapas acontecem em ordem, para que nada importante seja pulado.", "Rode `agentarch workflow status` e conclua a próxima etapa primeiro."),
	r("workflow.cannot_advance", `^cannot advance:`,
		"The next step is still missing the items listed.", "Complete each listed item, one at a time, then try again.",
		"A próxima etapa ainda não tem os itens listados.", "Complete cada item listado, um de cada vez, e tente de novo."),
	r("workflow.task_phase", `^task completion is recorded during implementing`,
		"Tasks are marked done only while building.", "Checkpoint implementing first.",
		"Tarefas só são marcadas como feitas durante a construção.", "Faça o checkpoint implementing primeiro."),
	r("workflow.unknown_task", `^unknown task id`,
		"No task has that id.", "Run `agentarch workflow next` to see the ready tasks.",
		"Nenhuma tarefa tem esse id.", "Rode `agentarch workflow next` para ver as tarefas prontas."),
	r("workflow.task_dependency", `^task \S+ (depends on incomplete|is incomplete)`,
		"A task this one depends on is not finished, or a task is still open.", "Finish the tasks it depends on first.",
		"Uma tarefa da qual esta depende ainda não terminou, ou uma tarefa continua aberta.", "Termine primeiro as tarefas das quais ela depende."),
	r("workflow.review_bounds", `^review (exceeds bounded|kind must be|verdict must be|summary must describe)`,
		"The review record is incomplete or malformed.", "Record the reviewer, kind (human or independent_agent), verdict (approved or changes_requested) and a short summary of what was checked.",
		"O registro da revisão está incompleto ou mal formado.", "Registre o revisor, o tipo (human ou independent_agent), o veredito (approved ou changes_requested) e um resumo curto do que foi conferido."),
	r("workflow.verify_before_review", `^successful verification is required before review`,
		"A review only counts after the checks pass.", "Run `agentarch workflow verify` and fix what fails.",
		"Uma revisão só vale depois que as verificações passam.", "Rode `agentarch workflow verify` e corrija o que falhar."),
	r("workflow.self_review", `^reviewer must be identified and independent`,
		"Whoever built the change cannot approve it. That is what makes the review worth something.", "Ask another person, or run the independent reviewer agent in a fresh session and record it as independent_agent.",
		"Quem construiu a mudança não pode aprová-la. É isso que dá valor à revisão.", "Peça a outra pessoa, ou rode o agente revisor independente numa sessão nova e registre como independent_agent."),
	r("workflow.stale_evidence", `^verification evidence is stale or failed`,
		"The code changed after the checks ran, or the checks did not pass.", "Run `agentarch workflow verify` again.",
		"O código mudou depois das verificações, ou elas não passaram.", "Rode `agentarch workflow verify` de novo."),
	r("workflow.commit_first", `^(commit the product before approving|commit source changes, then verify)`,
		"Approval is tied to an exact saved version (a commit), and there are unsaved changes.", "Commit the changes, then verify and review that exact commit.",
		"A aprovação fica presa a uma versão salva exata (um commit), e há mudanças não salvas.", "Faça o commit das mudanças e depois verifique e revise esse commit exato."),
	r("workflow.verify_prereq", `^(implementation and task checkpoints are required before verification|verification prerequisites:)`,
		"Verification needs the building step recorded first, and every planned task done.", "Checkpoint each finished task and the implementing step, then verify.",
		"A verificação precisa da etapa de construção registrada e de todas as tarefas planejadas concluídas.", "Faça o checkpoint de cada tarefa concluída e da etapa implementing, depois verifique."),
	r("workflow.universal_checks", `^(universal profile requires explicit checks|custom checks |check id .* is reserved|check program must be|check timeout_seconds)`,
		"For a technology without a built-in profile, the checks must be declared in product.yaml, and the security checks cannot be replaced.", "Ask your assistant to propose checks for format, lint, tests and build for your stack; approve them in an ADR.",
		"Para uma tecnologia sem perfil pronto, as verificações precisam ser declaradas no product.yaml, e as de segurança não podem ser substituídas.", "Peça ao seu assistente para propor verificações de formatação, lint, testes e build para a sua stack; aprove-as num ADR."),
	r("workflow.pr_record", `^(configure origin before recording|PR URL must|URL must identify a pull request|PR must reference the exact|PR source worktree must be clean|.* must be a plain HTTPS URL)`,
		"The pull request being recorded does not match this repository or the reviewed commit.", "Publish the PR from the reviewed commit in this repository's origin, then record its HTTPS address.",
		"O pull request registrado não corresponde a este repositório ou ao commit revisado.", "Publique o PR a partir do commit revisado na origin deste repositório e registre o endereço HTTPS."),
	r("workflow.release_gate", `^(a release needs passed verification|release --head must be|release source worktree must be clean|the released source differs|environment must be a short)`,
		"A release can only be recorded for exactly what was verified and reviewed, checked out here.", "Check out the deployed commit, make sure it matches the reviewed one, and record again.",
		"Um release só pode ser registrado para exatamente o que foi verificado e revisado, com esse commit aberto aqui.", "Abra o commit publicado, confira se é o revisado e registre de novo."),
	r("workflow.release_plan", `^release plan incomplete`,
		"release.md is missing sections a safe release needs.", "Fill environment, owner, rollback, post-deploy verification and signals in agentarch/project/release.md.",
		"Faltam no release.md seções que um release seguro precisa.", "Preencha ambiente, responsável, rollback, verificação pós-deploy e sinais em agentarch/project/release.md."),
	r("workflow.production_check", `^(attester must name|outcome must be|record a release before|the source changed after release|summary must say what was checked)`,
		"The production check record is incomplete or does not match the release.", "Say who checked, passed or failed, what was observed, and link the dashboard or log.",
		"O registro da verificação em produção está incompleto ou não corresponde ao release.", "Diga quem verificou, se passou ou falhou, o que foi observado, e o link do painel ou log."),
}

var acceptanceRefusals = []refusal{
	r("workflow.owner_acceptance", `^(owner must name who tried|acceptance verdict must be|acceptance summary must say|owner acceptance is recorded after)`,
		"Your acceptance is recorded only after the checks pass, and it needs your name, accepted or rejected, and what you tried and saw.", "Try the product, then run `agentarch workflow accept --owner \"<your name>\" --verdict accepted|rejected --summary \"<what you tried and saw>\"`.",
		"O seu aceite só é registrado depois que as verificações passam, e precisa do seu nome, accepted ou rejected, e do que você testou e viu.", "Teste o produto e rode `agentarch workflow accept --owner \"<seu nome>\" --verdict accepted|rejected --summary \"<o que você testou e viu>\"`."),
}

func init() { refusals = append(refusals, acceptanceRefusals...) }

func lookupRefusal(message string) (refusal, bool) {
	for _, x := range refusals {
		if x.Pattern.MatchString(message) {
			return x, true
		}
	}
	return refusal{}, false
}

// explainRefusal writes the plain-language explanation for an error, if it has one.
func explainRefusal(w io.Writer, err error) {
	if err == nil {
		return
	}
	x, ok := lookupRefusal(err.Error())
	if !ok {
		return
	}
	text, ok := x.Text[lang]
	if !ok {
		text = x.Text["en"]
	}
	label := map[string][2]string{"en": {"What this means:", "Next step:"}, "pt-BR": {"O que significa:", "Próximo passo:"}}[lang]
	if label[0] == "" {
		label = [2]string{"What this means:", "Next step:"}
	}
	fmt.Fprintf(w, "  %s %s\n  %s %s\n  (agentarch explain %s)\n", label[0], text[0], label[1], text[1], x.ID)
}

// explainRefusalID prints one entry for `agentarch explain workflow.<id>`.
func explainRefusalID(w io.Writer, id string) bool {
	for _, x := range refusals {
		if x.ID == id {
			text, ok := x.Text[lang]
			if !ok {
				text = x.Text["en"]
			}
			fmt.Fprintf(w, "%s\n\n%s\n\n%s\n", x.ID, text[0], text[1])
			return true
		}
	}
	return false
}
