import api from "./client.ts";

export async function getSecurityState() {
  return (
    await api.get<{ email: string; recovery_enabled: boolean }>("/v1/security")
  ).data;
}

export async function createRecoveryCode(current_password: string) {
  return (
    await api.post<{ recovery_code: string }>("/v1/security/recovery-code", {
      current_password,
    })
  ).data;
}

export async function changePassword(
  current_password: string,
  password: string,
) {
  await api.post("/v1/security/password", { current_password, password });
}

export async function recoverAccess(
  email: string,
  recovery_code: string,
  password: string,
) {
  await api.post("/auth/recover", { email, recovery_code, password });
}

export async function deleteAccount(
  current_password: string,
  confirmation: string,
) {
  await api.delete("/v1/security/account", {
    data: { current_password, confirmation },
  });
}
