# Esterium ERP — Homologação de acesso sem e-mail e recuperação presencial

**Escopo:** PR #49 (nome de usuário opcional ao e-mail) e PR #51 (recuperação de senha, issue #50).  
**Status:** roteiro de verificação, não evidência de execução nem autorização para produção.  
**Ambiente:** somente homologação, com empresas e funcionários fictícios. Nunca executar este roteiro no banco fiscal/operacional real.

## Pré-requisitos

- PRs aplicados em ordem e migrações 0050/0051 executadas, sem erro de aplicação.
- Administrador autorizado de uma empresa fictícia e um funcionário **caixa** fictício, com nome de usuário global único e sem e-mail.
- Outro CNPJ fictício isolado para testes de acesso negado.
- Acesso ao CI e logs de auditoria com controle de permissão. Não copiar senhas, JWT, links ou tokens para tickets, vídeos, screenshots, logs ou histórico do terminal.
- Browser atualizado, sessão do administrador separada da sessão do funcionário. Quando o mesmo computador for usado, encerrar a sessão entre os perfis.

## Matriz manual de aceite

| Verificação | Procedimento | Resultado esperado |
|---|---|---|
| Convite sem e-mail | Em Funcionários, criar um caixa usando somente nome de usuário. Abrir o link recebido e criar a senha pessoal. | O usuário ativa a conta, sem e-mail obrigatório, e acessa apenas sua loja como caixa. |
| Compatibilidade | Entrar normalmente com a conta preexistente por e-mail. | O login continua disponível, sem migração manual de usuário. |
| Emissão presencial | Entrar como administrador e usar **Recuperar senha** no caixa. | O link é exibido ao administrador uma única vez, tem validade de 30 minutos e não revela a nova senha. |
| Link no browser | Abrir o link de recuperação e observar a barra de endereços. | O fragmento com o token desaparece do endereço; o formulário permite definir e confirmar a nova senha. |
| Mudança de senha | Completar a recuperação. Entrar com nome de usuário e a nova senha. | Nova senha funciona, senha antiga deixa de autenticar. A função permanece caixa. |
| Sessões antigas | Antes da redefinição, abrir uma sessão do funcionário em outro navegador; após redefinição, tentar usar e renovar os tokens anteriores. | Access/refresh JWT antigos são rejeitados; nenhum privilégio é concedido. |
| Reuso | Tentar usar o mesmo link após a conclusão. | Operação recusada com mensagem genérica. A senha atual não muda. |
| Substituição | Emitir dois links sequenciais para o mesmo funcionário. | O primeiro torna-se inválido, somente o segundo ainda pode ser usado. |
| Revogação após recarregar | Emitir um link, recarregar a página Funcionários, abrir os links pendentes e cancelar a solicitação. | O link é cancelável, sem ser revelado novamente; a senha anterior permanece válida. |
| Expiração | Em banco de teste isolado, simular vencimento do link, sem manipular relógios do PDV. | Link vencido é rejeitado. O administrador consegue cancelá-lo na listagem. |
| Suspensão | Suspender o funcionário e, sob autorização do responsável, redefinir a senha. | O vínculo continua suspenso: trocar a senha não reativa o acesso. |
| Proteção admin | Tentar criar link para si próprio e para outro administrador. | Serviço rejeita ambas as ações. |
| Mudança de função | Emitir link para caixa e então promovê-lo a administrador **somente no ambiente de teste controlado**. | Link antigo passa a ser rejeitado. |
| Separação por CNPJ | Tentar emitir, listar ou cancelar reset de um empregado do CNPJ A a partir do CNPJ B. | Nenhum link, identificador secreto ou dado pessoal do outro CNPJ é exposto. |
| Associação posterior | Emitir link e vincular a identidade a outro CNPJ (somente teste). | O link deixa de funcionar porque a credencial é global. |
| Entradas inválidas | Testar link ausente/malformado/usado, senha curta e senha Unicode excessiva. | Erros não revelam estado da identidade; regras de 12 caracteres e 72 bytes UTF-8 são respeitadas. |
| Concorrência | Executar teste automatizado de integração de duas tentativas sobre o mesmo link. | Exatamente uma operação é concluída e a versão de credencial aumenta uma vez. |

## Critérios para retirar o rascunho do PR

1. Último commit do PR #51 com CI integral aprovado, incluindo **backend, frontend, security, integration, e2e e e2e-prodlike**.
2. Executar e registrar a jornada de navegador deste roteiro em homologação. O fato de a suíte E2E geral passar não comprova, por si só, a jornada específica de recuperação.
3. Conferir trilha de auditoria da emissão, conclusão e revogação, sem armazenamento ou exposição de segredos.
4. Revisar com responsável técnico os dados históricos, o rollback das migrações, o isolamento entre empresas e o comportamento após reinício dos serviços.
5. Não colocar recursos de recuperação em produção antes da revisão final e do merge autorizado.

**Observação:** a migração 0051 impede reversão que revalidaria JWTs revogados ou perderia links ativos/históricos. Nenhuma execução de rollback deve ser forçada em banco de produção.

**Pendência separada:** a mudança para execução local sem instalação de Go ou Docker (#44) ainda não começou e não é coberta por este roteiro.
