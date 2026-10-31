import { z } from "zod";

const email = z.string().trim().email("Enter a valid email address.");
export const loginSchema = z.object({
  email,
  password: z.string().min(1, "Enter your password."),
});
export const registerSchema = z.object({
  email,
  password: z
    .string()
    .refine(
      (value) => Array.from(value).length >= 8,
      "Use at least 8 characters.",
    )
    .refine(
      (value) => new TextEncoder().encode(value).length <= 72,
      "This password is too long. Try a shorter one.",
    ),
});
export type AuthFormValues = z.infer<typeof loginSchema>;
