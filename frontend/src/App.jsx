import { Suspense, lazy } from "react";
import { Route, Routes, Navigate } from "react-router-dom";
import { BackendProvider } from "./hooks/backend.jsx";
import { AppLayout, BareLayout } from "./components/Layout.jsx";

// Every page is its own chunk: the home page does not pay for the chaos lab.
const Home = lazy(() => import("./pages/Home.jsx"));
const Event = lazy(() => import("./pages/Event.jsx"));
const Lot = lazy(() => import("./pages/Lot.jsx"));
const Account = lazy(() => import("./pages/Account.jsx"));
const Wallet = lazy(() => import("./pages/Wallet.jsx"));
const Bids = lazy(() => import("./pages/Bids.jsx"));
const Sell = lazy(() => import("./pages/Sell.jsx"));
const Search = lazy(() => import("./pages/Search.jsx"));
const Status = lazy(() => import("./pages/Status.jsx"));
const Admin = lazy(() => import("./pages/Admin.jsx"));
const Tour = lazy(() => import("./pages/Tour.jsx"));
const Trust = lazy(() => import("./pages/Trust.jsx"));
const Chaos = lazy(() => import("./pages/Chaos.jsx"));
const System = lazy(() => import("./pages/System.jsx"));
const Race = lazy(() => import("./pages/Race.jsx"));
const SearchLab = lazy(() => import("./pages/SearchLab.jsx"));
const Formats = lazy(() => import("./pages/Formats.jsx"));

const Loading = () => <div className="boot" role="status" aria-label="Loading"><div className="skel" style={{ height: 64 }} /></div>;

export default function App() {
  return (
    <BackendProvider>
      <Suspense fallback={<Loading />}>
        <Routes>
          <Route element={<BareLayout />}>
            <Route path="/" element={<Home />} />
            <Route path="/event/:slug" element={<Event />} />
          </Route>
          <Route element={<AppLayout />}>
            <Route path="/lot/:id" element={<Lot />} />
            <Route path="/account" element={<Account />} />
            <Route path="/wallet" element={<Wallet />} />
            <Route path="/bids" element={<Bids />} />
            <Route path="/sell" element={<Sell />} />
            <Route path="/search" element={<Search />} />
            <Route path="/status" element={<Status />} />
            <Route path="/admin" element={<Admin />} />
            <Route path="/tour" element={<Tour />} />
            <Route path="/trust" element={<Trust />} />
            <Route path="/chaos" element={<Chaos />} />
            <Route path="/system" element={<System />} />
            <Route path="/race" element={<Race />} />
            <Route path="/search-lab" element={<SearchLab />} />
            <Route path="/formats" element={<Formats />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
      </Suspense>
    </BackendProvider>
  );
}
