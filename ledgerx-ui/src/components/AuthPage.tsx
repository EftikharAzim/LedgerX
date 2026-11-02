import { useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { isCancel } from "axios";
import { Link, Navigate, useSearchParams } from "react-router-dom";
import { login, register as registerAccount } from "../api/endpoints";
import { useAuth } from "../app/auth-context";
import { apiError } from "../lib/errors";
import {
  loginSchema,
  registerSchema,
  type AuthFormValues,
} from "../lib/auth-form";
import Icon, { BrandMark } from "./Icon";

export default function AuthPage({ signup = false }: { signup?: boolean }) {
  const [params] = useSearchParams();
  const restoreIntent = params.get("next") === "restore";
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<AuthFormValues>({
    resolver: zodResolver(signup ? registerSchema : loginSchema),
    mode: "onTouched",
  });
  const { isAuthed, login: startSession } = useAuth();
  const [visible, setVisible] = useState(false);
  const [serverError, setServerError] = useState("");
  const attempt = useRef<AbortController | null>(null);
  useEffect(() => () => attempt.current?.abort(), []);
  if (isAuthed)
    return (
      <Navigate
        to={restoreIntent ? "/workspace" : signup ? "/welcome" : "/"}
        replace
      />
    );

  const submit = handleSubmit(async (values) => {
    setServerError("");
    const controller = new AbortController();
    attempt.current = controller;
    try {
      const tokens = await (signup ? registerAccount : login)(
        values.email,
        values.password,
        controller.signal,
      );
      if (controller.signal.aborted) return;
      startSession(tokens);
    } catch (error) {
      if (!isCancel(error) && !controller.signal.aborted) {
        const message = apiError(error);
        setServerError(
          message === "invalid credentials"
            ? "That email and password don't match. Please try again."
            : message === "email already registered"
              ? "An account already uses this email. Log in to continue."
              : message,
        );
      }
    }
  });

  return (
    <div className="auth-page auth-compact">
      <main className="auth-main">
        <div className="auth-mobile-brand">
          <BrandMark />
          <span>LedgerX</span>
        </div>
        <div className="auth-top-link">
          <span>{signup ? "Already have an account?" : "New to LedgerX?"}</span>
          <Link
            to={
              (signup ? "/login" : "/register") +
              (restoreIntent ? "?next=restore" : "")
            }
          >
            {signup ? "Log in" : "Create an account"}
            <Icon name="arrow" width="15" height="15" />
          </Link>
        </div>
        <div className="auth-form-container">
          <h1>{signup ? "Create account" : "Log in"}</h1>
          {!signup &&
            (params.has("recovered") || params.has("password_changed")) && (
              <p role="status" className="success-notice">
                Your password has changed. Log in again. If you used a recovery
                code, save a new one in Account & security.
              </p>
            )}
          {!signup && params.has("deleted") && (
            <p role="status" className="success-notice">
              Your account and live financial records were deleted. Background
              report and event cleanup will continue on the server.
            </p>
          )}
          {!signup && (
            <p>
              <Link className="text-link" to="/recover">
                Forgot your password?
              </Link>
            </p>
          )}
          <form
            className="auth-form"
            noValidate
            onSubmit={submit}
            onChange={() => setServerError("")}
          >
            <div>
              <label className="label" htmlFor="auth-email">
                Email address
              </label>
              <input
                id="auth-email"
                className="input"
                type="email"
                autoComplete="email"
                autoCapitalize="none"
                spellCheck={false}
                placeholder="you@example.com"
                disabled={isSubmitting}
                aria-invalid={!!errors.email}
                aria-describedby={errors.email ? "email-error" : undefined}
                {...register("email")}
              />
              {errors.email && (
                <p id="email-error" role="alert" className="field-error">
                  {errors.email.message}
                </p>
              )}
            </div>
            <div>
              <label className="label" htmlFor="auth-password">
                Password
              </label>
              <div className="password-field">
                <input
                  id="auth-password"
                  className="input"
                  type={visible ? "text" : "password"}
                  autoComplete={signup ? "new-password" : "current-password"}
                  placeholder={
                    signup ? "At least 8 characters" : "Enter your password"
                  }
                  disabled={isSubmitting}
                  aria-invalid={!!errors.password}
                  aria-describedby={
                    errors.password
                      ? "password-error"
                      : signup
                        ? "password-hint"
                        : undefined
                  }
                  {...register("password")}
                />
                <button
                  type="button"
                  className="password-toggle"
                  aria-label={visible ? "Hide password" : "Show password"}
                  aria-pressed={visible}
                  aria-controls="auth-password"
                  onClick={() => setVisible(!visible)}
                >
                  <Icon
                    name={visible ? "eyeOff" : "eye"}
                    width="19"
                    height="19"
                  />
                </button>
              </div>
              {errors.password ? (
                <p id="password-error" role="alert" className="field-error">
                  {errors.password.message}
                </p>
              ) : (
                signup && (
                  <p id="password-hint" className="input-hint">
                    Use at least 8 characters and a password you don’t use
                    elsewhere.
                  </p>
                )
              )}
            </div>
            {serverError && (
              <p role="alert" className="form-error">
                <Icon name="info" width="18" height="18" />
                {serverError}
              </p>
            )}
            <button
              className="btn btn-primary auth-submit"
              disabled={isSubmitting}
            >
              {isSubmitting
                ? signup
                  ? "Creating account…"
                  : "Logging in…"
                : signup
                  ? "Create account"
                  : "Log in"}
              <Icon name="arrow" width="18" height="18" />
            </button>
          </form>
        </div>
      </main>
    </div>
  );
}
