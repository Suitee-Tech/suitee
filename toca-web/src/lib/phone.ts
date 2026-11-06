// Regras compartilhadas com o backend (`pkg/validation/phone.go`).
// O formulário do site precisa recusar o mesmo telefone que o CRM recusaria;
// validar apenas o comprimento deixava passar celular sem o 9 e produzia um
// lead local "failed" com resposta de sucesso para o comprador.
const BR_AREA_CODES = new Set([
  '11','12','13','14','15','16','17','18','19',
  '21','22','24','27','28',
  '31','32','33','34','35','37','38',
  '41','42','43','44','45','46','47','48','49',
  '51','53','54','55',
  '61','62','63','64','65','66','67','68','69',
  '71','73','74','75','77','79',
  '81','82','83','84','85','86','87','88','89',
  '91','92','93','94','95','96','97','98','99',
])

export function onlyDigits(value: string): string {
  return (value ?? '').replace(/\D/g, '')
}

/** Normaliza para DDD + assinante (10 ou 11 dígitos), removendo +55. */
export function nationalPhoneDigits(value: string): string {
  let digits = onlyDigits(value)
  if (digits.length > 11 && digits.startsWith('55')) digits = digits.slice(2)
  return digits.slice(0, 11)
}

/** Devolve a mensagem do primeiro problema ou null quando o telefone é válido. */
export function validateBrazilianPhone(value: string): string | null {
  const digits = nationalPhoneDigits(value)
  if (!digits) return 'Informe o telefone'
  if (digits.length !== 10 && digits.length !== 11) {
    return 'Telefone deve ter DDD + número (10 ou 11 dígitos)'
  }
  if (!BR_AREA_CODES.has(digits.slice(0, 2))) {
    return 'DDD inválido — confira o código da sua região'
  }
  if (digits.length === 11 && digits[2] !== '9') {
    return 'Celular deve começar com 9 depois do DDD'
  }
  if (digits.length === 10) {
    const first = digits[2]
    if (first === '9') return 'Faltou um dígito no celular'
    if (first < '2' || first > '5') {
      return 'Telefone fixo deve começar com 2, 3, 4 ou 5 depois do DDD'
    }
  }
  return null
}
