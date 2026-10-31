import { useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { isCancel } from "axios";
import { Link, Navigate } from "react-router-dom";
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
  if (isAuthed) return <Navigate to={signup ? "/welcome" : "/"} replace />;

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
    <div className="auth-page">
      <aside className="auth-story">
        <Link
          to="/login"
          className="brand auth-brand"
          aria-label="LedgerX home"
        >
          <BrandMark />
          <span>
            Ledger<span className="brand-x">X</span>
          </span>
        </Link>
        <div className="auth-story-copy">
          <p className="eyebrow">YOUR MONEY. IN BALANCE.</p>
          <h2>
            A little clarity.
            <br />
            <em>A world of possibility.</em>
          </h2>
          <p>
            Give your everyday finances a little structure.
            <br />
            Make room for what matters to you.
          </p>
          <div className="auth-ledger-art" aria-hidden="true">
            <div className="auth-orbit" />
            <Icon
              name="sparkle"
              className="auth-spark"
              width="46"
              height="46"
            />
            <div className="auth-art-card">
              <span>
                <Icon name="wallet" width="21" height="21" />A place for every
                purpose
              </span>
              <div className="art-ledger-lines">
                <i />
                <i />
                <i />
              </div>
              <span className="art-balanced">
                <Icon name="check" width="16" height="16" />
                Every entry has its equal.
              </span>
            </div>
          </div>
          <ul className="auth-features">
            <li>
              <Icon name="wallet" />
              <span>
                <strong>Keep your accounts together.</strong>
                <span>Different goals. One organized workspace.</span>
              </span>
            </li>
            <li>
              <Icon name="receipt" />
              <span>
                <strong>A record that stays complete.</strong>
                <span>Income, expenses, transfers, and corrections.</span>
              </span>
            </li>
            <li>
              <Icon name="export" />
              <span>
                <strong>Take your records with you.</strong>
                <span>Monthly CSV exports, whenever you need them.</span>
              </span>
            </li>
          </ul>
        </div>
        <span className="auth-story-footer">A calmer way to keep track.</span>
      </aside>
      <main className="auth-main">
        <div className="auth-mobile-brand">
          <BrandMark />
          <span>LedgerX</span>
        </div>
        <div className="auth-top-link">
          <span>{signup ? "Already have an account?" : "New to LedgerX?"}</span>
          <Link to={signup ? "/login" : "/register"}>
            {signup ? "Log in" : "Create an account"}
            <Icon name="arrow" width="15" height="15" />
          </Link>
        </div>
        <div className="auth-form-container">
          <span className="auth-welcome-symbol">
            <Icon name={signup ? "sparkle" : "wallet"} width="25" height="25" />
          </span>
          <p className="eyebrow">
            {signup ? "YOUR NEXT CHAPTER" : "RIGHT WHERE YOU LEFT OFF"}
          </p>
          <h1>
            {signup ? (
              <>
                Make yourself <em>at home.</em>
              </>
            ) : (
              <>
                Good to have
                <br />
                you <em>back.</em>
              </>
            )}
          </h1>
          <p className="auth-form-intro">
            {signup
              ? "Create your workspace. Start with a little clarity."
              : "Log in for a clearer view of your everyday finances."}
          </p>
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
                    At least 8 characters. Make it something only you know.
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
                  ? "Creating your workspace…"
                  : "Logging in…"
                : signup
                  ? "Create account"
                  : "Log in"}
              <Icon name="arrow" width="18" height="18" />
            </button>
          </form>
          <div className="auth-form-note">
            <Icon name="shield" width="16" height="16" />
            <span>
              {signup
                ? "Your own workspace. One account at a time."
                : "Your workspace, ready when you are."}
            </span>
          </div>
        </div>
        <footer className="auth-footer">
          <span>LedgerX</span>
          <span>A little clarity goes a long way.</span>
        </footer>
      </main>
    </div>
  );
}
