# SignPath OSS — prontidão para assinatura do instalador Windows

**Estado em 05/11/2026:** caminho escolhido pelo usuário; inscrição, aprovação e integração ainda pendentes. Este arquivo é preparação local, não prova de aprovação pelo SignPath.

## O que a rota oferece

SignPath Foundation pode fornecer assinatura Authenticode gratuita para projetos open source aprovados. O certificado é emitido em nome da **SignPath Foundation**; essa identidade aparecerá como publicador, não “Esterium”. Cada release exige aprovação manual. A aceitação do projeto é decisão do provedor e não é garantida. Consulte os [termos oficiais da SignPath Foundation](https://signpath.org/terms.html).

O repositório contém licença MIT em `LICENSE`, mas isso sozinho não comprova elegibilidade do pacote inteiro: a SignPath exige licença open source aprovada e ausência de componentes proprietários em todos os componentes distribuídos. Também exige manutenção ativa, uma release funcional já publicada no mesmo formato que será assinada, política pública de assinatura, privacidade documentada, MFA para toda a equipe e papéis claros de autor, revisor e aprovador. A decisão de aceitar o projeto é discricionária.

## Estado atual e lacunas

- A única release pública é [`v0.0.0-unsigned-preview-20261024`](https://github.com/GustavoMFDS/SistemaERP/releases/tag/v0.0.0-unsigned-preview-20261024), marcada **“NÃO INSTALA”**. Não existe release pública da forma funcional que se pretende assinar.
- O builder agora embute os hashes aprovados e o thumbprint do publicador no EXE final. O instalador habilita `Aplicar` somente quando payload, metadados e assinatura Authenticode válida do publicador embutido conferem; o plano continua executando scripts com `AllSigned`. A prévia técnica mantém `Aplicar` desativado.
- A Actions atual usa certificado autoassinado temporário somente dentro do teste em runner descartável. Não há pedido de assinatura SignPath nem artefato funcional assinado para distribuição.
- Os campos de aprovação são preenchidos pelo build e ficam somente leitura; o usuário final informa apenas os dados variáveis da instalação.
- O build de release agora oferece `pgFormat=zip`: ele inclui o ZIP oficial do PostgreSQL, fixa seu SHA-256 e deixa `Apply` disponível somente quando payload, metadados e assinatura Authenticode do Setup forem válidos. A prévia técnica continua bloqueada. A CI E2E em Windows descartável para a nova rota ZIP ainda está pendente; antes de publicar, também falta revisar os avisos/licenças das dependências contidas no ZIP. A rota EDB continua disponível para a matriz de compatibilidade e ainda requer revisão própria se for distribuída.
- Existe um inventário técnico de dados em `docs/privacy/data-inventory.md`; ele próprio informa que não é garantia de conformidade jurídica. Ainda é preciso confirmar que a política de privacidade pública descreve o produto instalado e os destinos de dados configurados.
- README antes dizia que não havia licença, apesar do arquivo MIT. Corrigi essa inconsistência e acrescentei o cabeçalho com o termo “Code signing policy”; a política formal exigida pelo SignPath continua pendente de equipe/MFA confirmados, aprovação e publicação.

## Papéis propostos

- **Autor e mantenedor:** `GustavoMFDS`, proprietário visível do repositório.
- **Revisão:** mudanças de colaboradores sem função de committer devem ser revisadas antes de entrar na branch de release.
- **Aprovação de assinatura:** `GustavoMFDS` deve aprovar manualmente cada pedido após conferir commit de origem, CI, testes do instalador, hash do pacote e licenças dos fornecedores.
- **MFA:** todos os membros com acesso de escrita ao código ou à aprovação SignPath devem ativar MFA tanto no GitHub quanto no SignPath.

Esses papéis são uma proposta para configuração. A equipe e o proprietário devem confirmá-los antes de publicar a política final.

## Integração técnica pendente

1. Passar a CI E2E em Windows descartável para a instalação PostgreSQL ZIP e revisar os avisos/licenças das dependências incluídas antes da publicação.
2. Produzir e validar uma versão funcional no formato de distribuição pretendido.
3. Completar a política pública com equipe, aprovadores e privacidade; obter aceite da inscrição gratuita.
4. Configurar projeto, organização, políticas e configuração de artefato no SignPath.
5. Ligar um workflow de release à [ação oficial do SignPath](https://github.com/SignPath/github-action-submit-signing-request), guardada por tag/release e aprovação manual. O token pertence aos GitHub Actions Secrets; nunca deve ser enviado no chat nem commitado.
6. Assinar os executáveis e scripts Esterium e o Setup final com a mesma identidade aprovada; manter binários de fornecedores intactos e validados por hash/assinatura própria.
7. Verificar Authenticode, thumbprint, manifesto e hash do artefato; instalar e testar o pacote assinado num Windows limpo antes de distribuir.

Assinatura pública válida reduz o aviso de “publicador desconhecido”, mas não garante ausência de alertas SmartScreen em qualquer computador; o próprio Windows pode mostrar avisos enquanto a reputação do arquivo é nova. A Microsoft descreve esse comportamento em [opções de assinatura para apps Windows](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/code-signing-options).

## Próximo passo externo

O proprietário deve criar a conta SignPath, ativar MFA no SignPath e GitHub e submeter o projeto para avaliação. Depois da aprovação, a configuração pode ser ligada a Actions sem compartilhar token ou chave privada no chat. Nenhuma inscrição, alteração remota, publicação, assinatura ou release foi feita nesta preparação.
