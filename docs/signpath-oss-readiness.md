# SignPath OSS — prontidão para assinatura do instalador Windows

**Estado em 06/11/2026:** SignPath OSS escolhido; inscrição, aprovação e integração ainda pendentes. Este arquivo é preparação técnica, não prova de aprovação pelo SignPath.

## O que a rota oferece

SignPath Foundation pode fornecer assinatura Authenticode gratuita para projetos open source aprovados. O certificado é emitido em nome da **SignPath Foundation**; essa identidade aparecerá como publicador, não “Esterium”. Cada release exige aprovação manual. A aceitação do projeto é decisão do provedor e não é garantida. Consulte os [termos oficiais da SignPath Foundation](https://signpath.org/terms.html).

O repositório contém licença MIT em `LICENSE`, mas isso sozinho não comprova elegibilidade do pacote inteiro: a SignPath exige licença open source aprovada para todos os componentes e não permite componentes proprietários no pacote assinado. Também exige manutenção ativa, uma release funcional já publicada no mesmo formato que será assinado, política pública de assinatura, privacidade documentada, MFA para toda a equipe e papéis claros de autor, revisor e aprovador. A decisão de aceitar o projeto é discricionária.

## Estado atual e lacunas

- A única release pública é [`v0.0.0-unsigned-preview-20261024`](https://github.com/GustavoMFDS/SistemaERP/releases/tag/v0.0.0-unsigned-preview-20261024), marcada **“NÃO INSTALA”**. Não existe release pública da forma funcional que se pretende assinar.
- O builder agora embute os hashes aprovados e o thumbprint do publicador no EXE final. O instalador habilita `Aplicar` somente quando payload, metadados e assinatura Authenticode válida do publicador embutido conferem; o plano continua executando scripts com `AllSigned`. A prévia técnica mantém `Aplicar` desativado.
- A Actions atual usa certificado autoassinado temporário somente dentro do teste em runner descartável. Não há pedido de assinatura SignPath nem artefato funcional assinado para distribuição.
- Os campos de aprovação são preenchidos pelo build e ficam somente leitura; o usuário final informa apenas os dados variáveis da instalação.
- CI [42163167669](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42163167669) passou no head de código `fef5e418`. No head documental `e5c4d3c7`, [CI 42166884163](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42166884163) passou nos 20 jobs e [Windows Setup 42166884174](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42166884174) também passou: prévia, integração ZIP, primeira instalação/reparo/remoção com PostgreSQL EDB e ZIP e gate agregado. Quatro jobs exclusivos de `workflow_dispatch` foram ignorados. São runners descartáveis; o certificado temporário de teste não prova aprovação SignPath nem assinatura de publicador de produção.
- O artefato [esterium-windows-lab-screens](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42166884174/artifacts/13479483380) contém 76 capturas em grupos 01–19: 17 rotas principais, navegação/perfis e fluxos de mensagens/transações. Os artefatos [instalação/reparo/remoção com ZIP](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42166884174/artifacts/13479657049) e [EDB](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42166884174/artifacts/13479672330) vêm de CI Windows descartável, não da VM do usuário.
- O artefato [esterium-third-party-license-evidence-zip](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42166884174/artifacts/13479716871) e o log do laboratório ZIP registram 271 caminhos de avisos do arquivo PostgreSQL fixado: 195 em `pgsql/pgAdmin 4/python`, 74 em `pgsql/pgAdmin 4/runtime` e 2 em `pgsql/pgAdmin 4/docs`. O builder incorpora o ZIP oficial inteiro sem alteração, e o estágio extrai todas as entradas `pgsql/`; portanto o pgAdmin está no payload distribuído e preparado. O ERP não chama pgAdmin; a opção EDB existente pede servidor/utilitários de linha de comando e dispensa pgAdmin/StackBuilder. A classificação automática achou licenças variadas, inclusive GPL/LGPL, mas não decide os termos que se aplicam nem comprova conformidade. A revisão manual deve decidir entre validar a redistribuição desse subcomponente inteiro ou preparar um ZIP público sem pgAdmin e repetir os testes. O EXE EDB continua excluído da rota pública. Revisão humana/jurídica permanece aberta.
- Existe um inventário técnico de dados em `docs/privacy/data-inventory.md`; ele próprio informa que não é garantia de conformidade jurídica. Ainda é preciso confirmar que a política de privacidade pública descreve o produto instalado e os destinos de dados configurados.
- README antes dizia que não havia licença, apesar do arquivo MIT. Corrigi essa inconsistência e acrescentei o cabeçalho com o termo “Code signing policy”; a política formal exigida pelo SignPath continua pendente de equipe/MFA confirmados, aprovação e publicação.

## Revalidação CI em 06/11/2026 — head documental

- PR #216 continua aberta e Draft no head `e24dbc42903af586e3a46435755bebd573edd3c8`. CI [42169933570](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42169933570) passou nos **20/20 jobs**.
- Windows Setup [42169933573](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42169933573) passou em prévia unsigned, integração ZIP, primeira instalação/reparo/remoção por ZIP e EDB e gate agregado. Quatro jobs reservados a `workflow_dispatch` foram ignorados.
- Artefatos desta execução: [telas do laboratório](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42169933573/artifacts/13481636536), [instalação ZIP](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42169933573/artifacts/13480941923), [instalação EDB](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42169933573/artifacts/13480953317), [prévia técnica unsigned](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42169933573/artifacts/13480756947), e [avisos PostgreSQL ZIP](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42169933573/artifacts/13481535831) / [EDB](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42169933573/artifacts/13481117942). Expiram em 13/11/2026.
- Os gates e artefatos vieram de runners descartáveis. Não são prova de instalação na VM nem aprovação/assinatura SignPath.

## Papéis propostos

- **Autor e mantenedor:** `GustavoMFDS`, proprietário visível do repositório.
- **Revisão:** mudanças de colaboradores sem função de committer devem ser revisadas antes de entrar na branch de release.
- **Aprovação de assinatura:** `GustavoMFDS` deve aprovar manualmente cada pedido após conferir commit de origem, CI, testes do instalador, hash do pacote e licenças dos fornecedores.
- **MFA:** todos os membros com acesso de escrita ao código ou à aprovação SignPath devem ativar MFA tanto no GitHub quanto no SignPath.

Esses papéis são uma proposta para configuração. A equipe e o proprietário devem confirmá-los antes de publicar a política final.

## Integração técnica pendente

1. Concluir a revisão de licenças e redistribuição do ZIP PostgreSQL fixado e dos demais componentes, usando o inventário de 271 caminhos como evidência; a coleta automática não determina sozinha compatibilidade nem os avisos aplicáveis a cada binário.
2. Preparar e validar uma release funcional no formato público escolhido. A release pública atual é apenas prévia e informa que não instala.
3. Confirmar que a publicação pública exclui o instalador EDB EXE e inclui somente dependências com redistribuição autorizada.
4. Confirmar e publicar a política de assinatura, papéis, MFA e privacidade; depois submeter a inscrição gratuita e aguardar a decisão da SignPath.
5. Após aprovação, configurar projeto, organização, políticas e configuração do artefato no SignPath.
6. Ligar o workflow de release à [ação oficial do SignPath](https://github.com/SignPath/github-action-submit-signing-request), protegido por tag/release e aprovação manual. O token pertence aos GitHub Actions Secrets; nunca deve ser enviado no chat nem commitado.
7. Assinar executáveis e scripts Esterium e o Setup final com a identidade aprovada; manter binários de fornecedores intactos e validados por hash/assinatura própria.
8. Verificar Authenticode, thumbprint, manifesto e hash do artefato; instalar e testar o pacote assinado num Windows limpo antes de distribuir.

Assinatura pública válida reduz o aviso de “publicador desconhecido”, mas não garante ausência de alertas SmartScreen em qualquer computador; o próprio Windows pode mostrar avisos enquanto a reputação do arquivo é nova. A Microsoft descreve esse comportamento em [opções de assinatura para apps Windows](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/code-signing-options).

## Próximo passo externo

O proprietário precisará criar a conta SignPath, confirmar equipe/papéis, ativar MFA e submeter o projeto quando a release funcional, os avisos e a política estiverem prontos. Nenhuma inscrição, alteração remota, publicação, assinatura ou release foi feita nesta preparação. PR #216 continua Draft.
