export class RpcError extends Error {
  constructor(
    public status: number,
    public details: unknown,
    message: string
  ) {
    super(message)
    this.name = 'RpcError'
  }
}
