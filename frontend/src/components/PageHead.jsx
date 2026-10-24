export default function PageHead({ title, children, action }) {
  return (
    <div className="page-head">
      <div><h1>{title}</h1>{children ? <p>{children}</p> : null}</div>
      {action}
    </div>
  );
}
