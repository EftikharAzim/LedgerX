import assert from "node:assert/strict";
import { beforeEach, test } from "node:test";
import axios, { AxiosError, type AxiosResponse, type InternalAxiosRequestConfig } from "axios";
import api from "../src/api/client.ts";
import { getAccess, getRefresh, getSession, subscribeAuth } from "../src/api/tokens.ts";
import { endSession, startSession } from "../src/app/auth-session.ts";
import { queryClient } from "../src/app/queryClient.ts";
import { login, register as registerUser } from "../src/api/endpoints.ts";

const storage = new Map<string, string>();
Object.defineProperty(globalThis, "localStorage", {
  configurable: true,
  value: {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => { storage.set(key, value); },
    removeItem: (key: string) => { storage.delete(key); },
  },
});

const tokensA = { access_token: "access-A", refresh_token: "refresh-A" };
const tokensB = { access_token: "access-B", refresh_token: "refresh-B" };
const rotatedA = { access_token: "access-A-new", refresh_token: "refresh-A-new" };

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((accept, fail) => { resolve = accept; reject = fail; });
  return { promise, resolve, reject };
}

function response(config: InternalAxiosRequestConfig, data: unknown): AxiosResponse {
  return { config, data, status: 200, statusText: "OK", headers: {} };
}

function unauthorized(config: InternalAxiosRequestConfig): never {
  throw new AxiosError("expired token", "ERR_BAD_REQUEST", config, undefined, {
    ...response(config, "invalid token"), status: 401, statusText: "Unauthorized",
  });
}

beforeEach(() => {
  endSession();
  axios.defaults.adapter = async () => { throw new Error("Unexpected auth request"); };
  api.defaults.adapter = async () => { throw new Error("Unexpected API request"); };
});

test("user changes clear fresh private data and fetch the next user's accounts", async () => {
  startSession(tokensA);
  let requests = 0;
  api.defaults.adapter = async (config) => {
    requests++;
    return response(config, [{ owner: config.headers.Authorization }]);
  };
  const accounts = () => queryClient.fetchQuery({
    queryKey: ["accounts"], queryFn: async () => (await api.get("/v1/accounts")).data,
  });
  assert.deepEqual(await accounts(), [{ owner: "Bearer access-A" }]);
  endSession();
  assert.equal(queryClient.getQueryData(["accounts"]), undefined);
  startSession(tokensB);
  assert.deepEqual(await accounts(), [{ owner: "Bearer access-B" }]);
  assert.equal(requests, 2);
});

test("a delayed request from the previous user is canceled and cannot refill the cache", async () => {
  startSession(tokensA);
  const started = deferred<InternalAxiosRequestConfig>();
  const pending = deferred<AxiosResponse>();
  api.defaults.adapter = (config) => { started.resolve(config); return pending.promise; };
  const oldQuery = queryClient.fetchQuery({
    queryKey: ["accounts"], queryFn: async () => (await api.get("/v1/accounts")).data,
  });
  const rejected = assert.rejects(oldQuery);
  const oldConfig = await started.promise;
  const oldSession = getSession();
  startSession(tokensB);
  assert.equal(oldSession.signal.aborted, true);
  assert.equal(oldConfig.signal?.aborted, true);
  pending.resolve(response(oldConfig, [{ owner: "A" }]));
  await rejected;
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(queryClient.getQueryData(["accounts"]), undefined);
  assert.equal(getAccess(), tokensB.access_token);
});

test("concurrent 401 responses share one refresh within the same session", async () => {
  startSession(tokensA);
  const started = deferred<InternalAxiosRequestConfig>();
  const pending = deferred<AxiosResponse>();
  let refreshes = 0;
  axios.defaults.adapter = (config) => {
    refreshes++;
    started.resolve(config);
    return pending.promise;
  };
  api.defaults.adapter = async (config) => {
    if (config.headers.Authorization === "Bearer access-A") unauthorized(config);
    return response(config, config.headers.Authorization);
  };
  const requests = Promise.all([api.get("/v1/accounts"), api.get("/v1/categories")]);
  const refreshConfig = await started.promise;
  await new Promise((resolve) => setTimeout(resolve, 0));
  pending.resolve(response(refreshConfig, rotatedA));
  const results = await requests;
  assert.equal(refreshes, 1);
  assert.deepEqual(results.map((result) => result.data), ["Bearer access-A-new", "Bearer access-A-new"]);
  assert.equal(getRefresh(), rotatedA.refresh_token);
});

test("refresh completing after logout cannot restore credentials and revokes its successor", async () => {
  startSession(tokensA);
  const started = deferred<InternalAxiosRequestConfig>();
  const pending = deferred<AxiosResponse>();
  const revoked: string[] = [];
  axios.defaults.adapter = (config) => {
    if (config.url?.endsWith("/auth/logout")) {
      revoked.push(JSON.parse(config.data).refresh_token);
      return Promise.resolve(response(config, null));
    }
    started.resolve(config);
    return pending.promise;
  };
  api.defaults.adapter = async (config) => unauthorized(config);
  const request = api.get("/v1/accounts");
  const rejected = assert.rejects(request, (error) => axios.isCancel(error));
  const refreshConfig = await started.promise;
  endSession();
  pending.resolve(response(refreshConfig, rotatedA));
  await rejected;
  assert.equal(getAccess(), null);
  assert.equal(getRefresh(), null);
  assert.deepEqual(revoked, [rotatedA.refresh_token]);
});

test("refresh completing after a new login cannot overwrite that user's credentials", async () => {
  startSession(tokensA);
  const started = deferred<InternalAxiosRequestConfig>();
  const pending = deferred<AxiosResponse>();
  const revoked: string[] = [];
  axios.defaults.adapter = (config) => {
    if (config.url?.endsWith("/auth/logout")) {
      revoked.push(JSON.parse(config.data).refresh_token);
      return Promise.resolve(response(config, null));
    }
    started.resolve(config);
    return pending.promise;
  };
  api.defaults.adapter = async (config) => unauthorized(config);
  const request = api.get("/v1/accounts");
  const rejected = assert.rejects(request, (error) => axios.isCancel(error));
  const refreshConfig = await started.promise;
  startSession(tokensB);
  pending.resolve(response(refreshConfig, rotatedA));
  await rejected;
  assert.equal(getAccess(), tokensB.access_token);
  assert.equal(getRefresh(), tokensB.refresh_token);
  assert.deepEqual(revoked, [rotatedA.refresh_token]);
});

test("an earlier session's failed refresh cannot log the new user out", async () => {
  startSession(tokensA);
  const started = deferred<void>();
  const pending = deferred<AxiosResponse>();
  axios.defaults.adapter = () => { started.resolve(); return pending.promise; };
  api.defaults.adapter = async (config) => unauthorized(config);
  const request = api.get("/v1/accounts");
  const rejected = assert.rejects(request, (error) => axios.isCancel(error));
  await started.promise;
  startSession(tokensB);
  pending.reject(new Error("old refresh failed"));
  await rejected;
  assert.equal(getAccess(), tokensB.access_token);
  assert.equal(getRefresh(), tokensB.refresh_token);
});

test("refresh failure clears private data and notifies the route guard", async () => {
  startSession(tokensA);
  queryClient.setQueryData(["accounts"], [{ owner: "A" }]);
  let notifications = 0;
  const unsubscribe = subscribeAuth(() => { notifications++; });
  api.defaults.adapter = async (config) => unauthorized(config);
  axios.defaults.adapter = async () => { throw new Error("refresh expired"); };
  await assert.rejects(api.get("/v1/accounts"));
  unsubscribe();
  assert.equal(getAccess(), null);
  assert.equal(queryClient.getQueryData(["accounts"]), undefined);
  assert.equal(notifications, 1);
});

test("export visit history is cleared on logout and on a new login", () => {
  startSession(tokensA);
  queryClient.setQueryData(["export-jobs"], [{ id: 12, month: "2026-10" }]);
  endSession();
  assert.equal(queryClient.getQueryData(["export-jobs"]), undefined);
  startSession(tokensA);
  queryClient.setQueryData(["export-jobs"], [{ id: 13, month: "2026-09" }]);
  startSession(tokensB);
  assert.equal(queryClient.getQueryData(["export-jobs"]), undefined);
});

test("leaving either auth form cancels a delayed authentication response", async () => {
  for (const authenticate of [login, registerUser]) {
    const started = deferred<InternalAxiosRequestConfig>();
    const pending = deferred<AxiosResponse>();
    api.defaults.adapter = (config) => {
      started.resolve(config);
      return pending.promise;
    };
    const controller = new AbortController();
    const request = authenticate(
      "someone@example.com",
      "example-password",
      controller.signal,
    );
    const rejected = assert.rejects(request, (error) => axios.isCancel(error));
    const config = await started.promise;
    controller.abort();
    pending.resolve(response(config, rotatedA));
    await rejected;
    assert.equal(getAccess(), null);
  }
});
