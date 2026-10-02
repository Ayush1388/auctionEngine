// Seed lots for the mock API. Amounts are in paise (INR × 100), matching the
// real API's "smallest currency unit". Times are offsets from server start.
// Descriptions use the interim header convention from plan.md 3.3.

const MIN = 60_000;
const HOUR = 60 * MIN;
const DAY = 24 * HOUR;
const rupees = (n) => n * 100;

const header = (specs, body) =>
  Object.entries(specs)
    .map(([key, value]) => `${key}: ${value}`)
    .join("\n") +
  "\n\n" +
  body;

export const SEED = [
  // ----- live
  {
    name: "1970 Porsche 911 S 2.2 Coupé", type: "classic", status: "ACTIVE", endsIn: 2 * DAY + 4 * HOUR,
    start: rupees(2500000), increment: rupees(50000), bid: rupees(4200000), bids: 31, recent: 9,
    specs: { Photos: "porsche-911-1970", Make: "Porsche", Model: "911 S", Year: "1970", Mileage: "84,200 km", Engine: "2.2 L flat-six", Transmission: "5-speed manual", Fuel: "Petrol", Body: "Coupé", Location: "Pune, Maharashtra" },
    body: "Matching-numbers example finished in Tangerine over black leatherette. Mechanical fuel injection rebuilt in 2024.",
  },
  {
    name: "2023 Ferrari SF90 Stradale", type: "modern", status: "ACTIVE", endsIn: 5 * HOUR + 12 * MIN,
    start: rupees(45000000), increment: rupees(500000), bid: rupees(61000000), bids: 44, recent: 12,
    specs: { Photos: "ferrari-sf90", Make: "Ferrari", Model: "SF90 Stradale", Year: "2023", Mileage: "2,150 km", Engine: "4.0 L V8 hybrid", Transmission: "8-speed dual-clutch", Fuel: "Petrol hybrid", Body: "Coupé", Location: "Mumbai, Maharashtra" },
    body: "Single owner, Rosso Corsa over Nero. Assetto Fiorano package, full service history with the supplying dealer.",
  },
  {
    name: "1959 Volkswagen Type 2 Microbus", type: "classic", status: "ACTIVE", endsIn: 38 * MIN,
    start: rupees(900000), increment: rupees(25000), bid: rupees(1825000), bids: 78, recent: 7,
    specs: { Photos: "vw-t1-1959", Make: "Volkswagen", Model: "Type 2 (T1)", Year: "1959", Mileage: "55,000 km", Engine: "1.2 L flat-four", Transmission: "4-speed manual", Fuel: "Petrol", Body: "Van", Location: "Panaji, Goa" },
    body: "Fifteen-window bus restored in two-tone Sealing Wax Red and Beige Grey. Roof rack and safari windows fitted.",
  },
  {
    name: "2019 McLaren 720S Performance", type: "modern", status: "ACTIVE", endsIn: 18 * MIN + 40_000,
    start: rupees(18000000), increment: rupees(250000), bid: rupees(23500000), bids: 22, recent: 5,
    specs: { Photos: "mclaren-720s", Make: "McLaren", Model: "720S", Year: "2019", Mileage: "9,800 km", Engine: "4.0 L twin-turbo V8", Transmission: "7-speed dual-clutch", Fuel: "Petrol", Body: "Coupé", Location: "Bengaluru, Karnataka" },
    body: "Performance specification in Papaya Spark with carbon exterior packs one and two. Nose lift fitted.",
  },
  {
    name: "1972 Mercedes-Benz 280 SL Pagoda", type: "classic", status: "ACTIVE", endsIn: 4 * MIN + 12_000,
    start: rupees(5500000), increment: rupees(50000), bid: rupees(8850000), bids: 52, recent: 6,
    specs: { Photos: "mercedes-280sl", Make: "Mercedes-Benz", Model: "280 SL (W113)", Year: "1972", Mileage: "1,12,400 km", Engine: "2.8 L inline-six", Transmission: "4-speed automatic", Fuel: "Petrol", Body: "Roadster", Location: "New Delhi" },
    body: "Late-production Pagoda with both tops, silver over dark blue. Bare-metal respray completed in 2021.",
  },
  {
    name: "1984 Nissan Sunny B110 1200 Pickup", type: "classic", status: "ACTIVE", endsIn: 1 * MIN + 52_000,
    start: rupees(150000), increment: rupees(5000), bid: rupees(310000), bids: 14, recent: 3,
    specs: { Photos: "sunny-pickup", Make: "Nissan", Model: "Sunny B110", Year: "1984", Mileage: "67,000 km", Engine: "1.2 L inline-four", Transmission: "4-speed manual", Fuel: "Petrol", Body: "Pickup", Location: "Kochi, Kerala" },
    body: "Unrestored survivor in pale blue. Original engine, recent brakes and tyres.",
  },
  {
    name: "1965 Ford Mustang Fastback 289", type: "classic", status: "ACTIVE", endsIn: 1 * DAY + 2 * HOUR,
    start: rupees(3000000), increment: rupees(50000), bid: rupees(4650000), bids: 19, recent: 2,
    specs: { Photos: "mustang-1965", Make: "Ford", Model: "Mustang Fastback", Year: "1965", Mileage: "38,900 mi", Engine: "289 cu in V8", Transmission: "4-speed manual", Fuel: "Petrol", Body: "Fastback", Location: "Chennai, Tamil Nadu" },
    body: "A-code 2+2 fastback in Wimbledon White with blue stripes. Imported in 2018, registered and road legal.",
  },
  {
    name: "2021 Porsche 911 GT3 (992)", type: "modern", status: "ACTIVE", endsIn: 3 * DAY + 6 * HOUR,
    start: rupees(22000000), increment: rupees(250000), bid: rupees(24750000), bids: 9, recent: 1,
    specs: { Photos: "porsche-gt3-992", Make: "Porsche", Model: "911 GT3", Year: "2021", Mileage: "6,400 km", Engine: "4.0 L flat-six", Transmission: "6-speed manual", Fuel: "Petrol", Body: "Coupé", Location: "Hyderabad, Telangana" },
    body: "Manual GT3 in Shark Blue with the Clubsport package and carbon bucket seats.",
  },
  {
    name: "2018 Lamborghini Huracán Performante", type: "modern", status: "ACTIVE", endsIn: 6 * DAY + 3 * HOUR,
    start: rupees(26000000), increment: rupees(250000), bid: null, bids: 0, recent: 0,
    specs: { Photos: "lamborghini-huracan", Make: "Lamborghini", Model: "Huracán Performante", Year: "2018", Mileage: "11,300 km", Engine: "5.2 L V10", Transmission: "7-speed dual-clutch", Fuel: "Petrol", Body: "Coupé", Location: "Mumbai, Maharashtra" },
    body: "Verde Mantis over black Alcantara. Forged composite aero, recent major service.",
  },
  {
    name: "1991 Honda NSX", type: "classic", status: "ACTIVE", endsIn: 9 * HOUR + 30 * MIN,
    start: rupees(6000000), increment: rupees(100000), bid: rupees(7400000), bids: 12, recent: 2,
    specs: { Photos: "honda-nsx-1991", Make: "Honda", Model: "NSX (NA1)", Year: "1991", Mileage: "71,500 km", Engine: "3.0 L V6", Transmission: "5-speed manual", Fuel: "Petrol", Body: "Coupé", Location: "Bengaluru, Karnataka" },
    body: "Formula Red with black roof. Timing belt and water pump replaced in 2025.",
  },
  // ----- scheduled
  {
    name: "1967 Jaguar E-Type Series 1 Roadster", type: "classic", status: "NOT_ACTIVE", startsIn: 5 * HOUR, endsIn: 7 * DAY + 5 * HOUR,
    start: rupees(9500000), increment: rupees(100000), bid: null, bids: 0, recent: 0,
    specs: { Photos: "jaguar-etype-1967", Make: "Jaguar", Model: "E-Type Series 1", Year: "1967", Mileage: "61,200 mi", Engine: "4.2 L inline-six", Transmission: "4-speed manual", Fuel: "Petrol", Body: "Roadster", Location: "Jaipur, Rajasthan" },
    body: "Opalescent Silver Blue over navy leather. Heritage certificate confirms matching engine and gearbox.",
  },
  {
    name: "2022 McLaren Artura", type: "modern", status: "NOT_ACTIVE", startsIn: 1 * DAY, endsIn: 8 * DAY,
    start: rupees(21000000), increment: rupees(250000), bid: null, bids: 0, recent: 0,
    specs: { Photos: "mclaren-artura", Make: "McLaren", Model: "Artura", Year: "2022", Mileage: "3,900 km", Engine: "3.0 L V6 hybrid", Transmission: "8-speed dual-clutch", Fuel: "Petrol hybrid", Body: "Coupé", Location: "New Delhi" },
    body: "Flux Green with the Performance interior. Balance of manufacturer warranty.",
  },
  {
    name: "1988 BMW M3 (E30)", type: "classic", status: "NOT_ACTIVE", startsIn: 2 * DAY, endsIn: 9 * DAY,
    start: rupees(5200000), increment: rupees(50000), bid: null, bids: 0, recent: 0,
    specs: { Photos: "bmw-m3-e30-1988", Make: "BMW", Model: "M3 (E30)", Year: "1988", Mileage: "1,34,000 km", Engine: "2.3 L inline-four", Transmission: "5-speed manual", Fuel: "Petrol", Body: "Coupé", Location: "Pune, Maharashtra" },
    body: "Alpine White over anthracite cloth. Dogleg gearbox, original panels throughout.",
  },
  {
    name: "2020 Ferrari F8 Tributo", type: "modern", status: "NOT_ACTIVE", startsIn: 3 * DAY, endsIn: 10 * DAY,
    start: rupees(32000000), increment: rupees(250000), bid: null, bids: 0, recent: 0,
    specs: { Photos: "ferrari-f8", Make: "Ferrari", Model: "F8 Tributo", Year: "2020", Mileage: "7,700 km", Engine: "3.9 L twin-turbo V8", Transmission: "7-speed dual-clutch", Fuel: "Petrol", Body: "Coupé", Location: "Chennai, Tamil Nadu" },
    body: "Giallo Modena with carbon racing seats and front lift.",
  },
  // ----- completed
  {
    name: "1963 Chevrolet Corvette Sting Ray", type: "classic", status: "COMPLETED", endsIn: -1 * DAY,
    start: rupees(7000000), increment: rupees(100000), bid: rupees(11200000), bids: 47, recent: 0,
    specs: { Photos: "corvette-1963", Make: "Chevrolet", Model: "Corvette Sting Ray", Year: "1963", Mileage: "58,300 mi", Engine: "327 cu in V8", Transmission: "4-speed manual", Fuel: "Petrol", Body: "Coupé", Location: "Mumbai, Maharashtra" },
    body: "Split-window coupé in Sebring Silver.",
  },
  {
    name: "2017 Nissan GT-R Nismo", type: "modern", status: "COMPLETED", endsIn: -2 * DAY,
    start: rupees(14000000), increment: rupees(100000), bid: rupees(17900000), bids: 36, recent: 0,
    specs: { Photos: "nissan-gtr-nismo", Make: "Nissan", Model: "GT-R Nismo", Year: "2017", Mileage: "18,600 km", Engine: "3.8 L twin-turbo V6", Transmission: "6-speed dual-clutch", Fuel: "Petrol", Body: "Coupé", Location: "Hyderabad, Telangana" },
    body: "Pearl White with the Nismo carbon package.",
  },
  {
    name: "1969 Fiat 500 L", type: "classic", status: "COMPLETED", endsIn: -3 * DAY,
    start: rupees(600000), increment: rupees(10000), bid: rupees(1140000), bids: 29, recent: 0,
    specs: { Photos: "fiat-500-1969", Make: "Fiat", Model: "500 L", Year: "1969", Mileage: "43,000 km", Engine: "0.5 L twin", Transmission: "4-speed manual", Fuel: "Petrol", Body: "Saloon", Location: "Panaji, Goa" },
    body: "Restored in Positano Yellow with a fabric sunroof.",
  },
  {
    name: "2015 Audi R8 V10 Plus", type: "modern", status: "COMPLETED", endsIn: -4 * DAY,
    start: rupees(13500000), increment: rupees(100000), bid: null, bids: 0, recent: 0,
    specs: { Photos: "audi-r8-v10", Make: "Audi", Model: "R8 V10 Plus", Year: "2015", Mileage: "32,100 km", Engine: "5.2 L V10", Transmission: "7-speed dual-clutch", Fuel: "Petrol", Body: "Coupé", Location: "Kochi, Kerala" },
    body: "Daytona Grey with carbon side blades.",
  },
].map((lot) => ({ ...lot, description: header(lot.specs, lot.body) }));
