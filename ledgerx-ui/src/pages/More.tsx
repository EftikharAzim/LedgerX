import { Link } from "react-router-dom";
import Icon, { type IconName } from "../components/Icon";

const groups: {
  title: string;
  links: { to: string; label: string; icon: IconName }[];
}[] = [
  {
    title: "Planning",
    links: [
      { to: "/planned", label: "Planned activity", icon: "calendar" },
      { to: "/schedules", label: "Recurring bills & income", icon: "clock" },
    ],
  },
  {
    title: "Manage",
    links: [
      { to: "/accounts", label: "Accounts", icon: "wallet" },
      { to: "/categories", label: "Categories", icon: "tag" },
    ],
  },
  {
    title: "Data",
    links: [
      { to: "/imports", label: "Import CSV", icon: "download" },
      { to: "/exports", label: "Export CSV", icon: "export" },
      { to: "/workspace", label: "Backup & restore", icon: "shield" },
    ],
  },
  {
    title: "Account",
    links: [
      { to: "/security", label: "Account & security", icon: "shield" },
      { to: "/welcome", label: "Getting started", icon: "info" },
    ],
  },
];

export default function More() {
  return (
    <div className="more-page">
      <div className="page-heading">
        <h1>More</h1>
      </div>
      <div className="more-groups">
        {groups.map((group) => (
          <section key={group.title} aria-label={group.title}>
            <h2>{group.title}</h2>
            <ul>
              {group.links.map((link) => (
                <li key={link.to}>
                  <Link to={link.to}>
                    <Icon name={link.icon} />
                    <span>{link.label}</span>
                    <Icon name="chevron" width="16" height="16" />
                  </Link>
                </li>
              ))}
            </ul>
          </section>
        ))}
      </div>
    </div>
  );
}
