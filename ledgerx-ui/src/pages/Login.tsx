import { useForm } from "react-hook-form";
import { z } from "zod";
import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { login } from "../api/endpoints";
import { useAuth } from "../app/auth";
import { apiError } from "../lib/errors";

const schema = z.object({ email: z.string().email(), password: z.string().min(8) });
type Form = z.infer<typeof schema>;

export default function Login() {
  const { register: reg, handleSubmit, formState: { errors, isSubmitting } } =
    useForm<Form>({ resolver: zodResolver(schema) });
  const { login: setToken } = useAuth();
  const nav = useNavigate();
  const [serverError, setServerError] = useState("");

  return (
    <div className="mx-auto mt-24 max-w-sm">
      <div className="card space-y-4">
        <div>
          <h1 className="text-xl font-semibold">Welcome back</h1>
          <p className="text-sm text-slate-500">Log in to your LedgerX account.</p>
        </div>
        <form
          className="space-y-3"
          onSubmit={handleSubmit(async (f) => {
            setServerError("");
            try {
              const { token } = await login(f.email, f.password);
              setToken(token);
              nav("/");
            } catch (e) {
              setServerError(apiError(e, "Invalid email or password"));
            }
          })}
        >
          <div>
            <label className="label">Email</label>
            <input className="input" placeholder="you@example.com" autoComplete="email" {...reg("email")} />
            {errors.email && <p className="field-error">{errors.email.message}</p>}
          </div>
          <div>
            <label className="label">Password</label>
            <input className="input" type="password" placeholder="••••••••" autoComplete="current-password" {...reg("password")} />
            {errors.password && <p className="field-error">{errors.password.message}</p>}
          </div>
          {serverError && <p className="field-error">{serverError}</p>}
          <button disabled={isSubmitting} className="btn btn-primary w-full">
            {isSubmitting ? "Logging in…" : "Log in"}
          </button>
        </form>
        <p className="text-center text-sm text-slate-500">
          No account? <Link to="/register" className="font-medium text-indigo-600 hover:underline">Create one</Link>
        </p>
      </div>
    </div>
  );
}
