import { useForm } from "react-hook-form";
import { z } from "zod";
import { zodResolver } from "@hookform/resolvers/zod";
import { register as apiRegister } from "../api/endpoints";
import { useAuth } from "../app/auth";
import { useNavigate } from "react-router-dom";

const schema = z.object({ email: z.string().email(), password: z.string().min(6) });
type Form = z.infer<typeof schema>;

export default function Register() {
  const { register: reg, handleSubmit, formState: { errors, isSubmitting } } =
    useForm<Form>({ resolver: zodResolver(schema) });
  const { login: setToken } = useAuth();
  const nav = useNavigate();

  return (
    <div className="max-w-sm mx-auto mt-24 p-6 rounded-2xl border">
      <h1 className="text-xl font-semibold mb-4">Register</h1>
      <form onSubmit={handleSubmit(async (f) => {
        const { token } = await apiRegister(f.email, f.password);
        setToken(token);
        nav("/");
      })} className="space-y-3">
        <input className="input" placeholder="Email" {...reg("email")} />
        {errors.email && <p className="text-red-600 text-sm">{errors.email.message}</p>}
        <input className="input" placeholder="Password" type="password" {...reg("password")} />
        {errors.password && <p className="text-red-600 text-sm">{errors.password.message}</p>}
        <button disabled={isSubmitting} className="btn w-full">{isSubmitting ? "…" : "Register"}</button>
      </form>
      <style>{`.input{width:100%;padding:.6rem;border:1px solid #ddd;border-radius:.75rem}
      .btn{padding:.6rem;border-radius:.75rem;border:1px solid #222}`}</style>
    </div>
  );
}
