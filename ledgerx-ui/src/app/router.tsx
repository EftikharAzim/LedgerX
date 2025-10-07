import { createBrowserRouter, RouterProvider } from "react-router-dom";
import Register from "../pages/Register";
import Accounts from "../pages/Accounts";
import Login from "../pages/Login";
import AccountDetails from "../pages/AccountDetails";
import Exports from "../pages/Exports";
import ProtectedRoute from "../components/ProtectedRoute";

const router = createBrowserRouter([
  { path: "/login", element: <Login /> },
  { path: "/register", element: <Register /> },
  {
    path: "/",
    element: <ProtectedRoute><Accounts /></ProtectedRoute>,
  },
  {
    path: "/accounts/:id",
    element: <ProtectedRoute><AccountDetails /></ProtectedRoute>,
  },
  {
    path: "/exports",
    element: <ProtectedRoute><Exports /></ProtectedRoute>,
  },
]);

export default function AppRouter() { return <RouterProvider router={router} />; }