/** True when the account is bound to the EU managed cluster (cluster_id "eu"). */
export function isEuAccount(account: { cluster_id?: string }): boolean {
  return account.cluster_id === 'eu';
}
