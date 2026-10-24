// The home page: the hero slideshow and the lots it shows before the backend answers.

export const SLIDES = [
  { img: "/assets/hero.jpg",    alt: "Black 1963 Chevrolet Impala lowrider at dusk", title: "1963 Chevrolet Impala SS", lot: "Lot 001, no reserve", bid: 42500, pos: "50% 82%", tag: "A noir icon" },
  { img: "/assets/ivan.jpg",    alt: "Red 1951 Pontiac Chieftain convertible",       title: "1951 Pontiac Chieftain",   lot: "Lot 019, Miami",      bid: 64500, pos: "50% 35%", tag: "Sunlit survivor" },
  { img: "/assets/doina.jpg",   alt: "Red Excalibur roadster",                       title: "1969 Excalibur Roadster",  lot: "Lot 024, New York",   bid: 71000, pos: "50% 65%", tag: "Neo-classic" },
  { img: "/assets/theodor.jpg", alt: "Red 1965 Ford Mustang coupe",                  title: "1965 Ford Mustang Coupe",  lot: "Lot 026, London",     bid: 38000, pos: "50% 64%", tag: "Pony car" },
];

export const HOME_LOTS = {
  classic: [
    { id: "corvette", img: "/assets/corvette.jpg", alt: "White 1963 Chevrolet Corvette Sting Ray convertible", title: "1963 Chevrolet Corvette Sting Ray", spec: "327 V8, 4-speed, red interior", where: "Sydney, 41,200 km", bid: 118000, step: 1000, end: 5400, bids: 27, reserve: "Reserve met" },
    { id: "gullwing", img: "/assets/300sl.jpg", alt: "Red 1955 Mercedes-Benz 300 SL Gullwing with its doors raised", title: "1955 Mercedes-Benz 300 SL Gullwing", spec: "3.0 fuel-injected six, cream leather", where: "Stuttgart, 62,000 km", bid: 1380000, step: 10000, end: 64800, bids: 31, reserve: "Reserve not met", pos: "50% 55%" },
    { id: "etype", img: "/assets/etype.jpg", alt: "Blue 1964 Jaguar E-Type Series 1 coupe in profile", title: "1964 Jaguar E-Type Series 1 Coupe", spec: "3.8 straight-six, 4-speed, wire wheels", where: "Norwich, 58,300 km", bid: 168000, step: 2000, end: 28800, bids: 24, reserve: "Reserve met", pos: "42% 62%" },
    { id: "porsche", img: "/assets/porsche.jpg", alt: "Cream 1972 Porsche 911 Targa on a country road", title: "1972 Porsche 911 S Targa", spec: "2.4 flat-six, 5-speed, matching numbers", where: "Lippe, 63,800 km", bid: 186000, step: 2000, end: 43200, bids: 41, reserve: "Reserve met" },
  ],
  muscle: [
    { id: "camaro", img: "/assets/camaro.jpg", alt: "Black 1967 Chevrolet Camaro SS convertible with white stripes", title: "1967 Chevrolet Camaro SS 350", spec: "350 V8, white stripes, convertible", where: "Heidelberg, 52,000 km", bid: 78500, step: 1000, end: 7200, bids: 37, reserve: "Reserve met", pos: "50% 60%" },
    { id: "gt500", img: "/assets/gt500.jpg", alt: "Black 1967 Shelby GT500 KR fastback", title: "1967 Shelby GT500 KR", spec: "428 Police Interceptor V8, fastback", where: "Nuremberg, 68,000 km", bid: 214000, step: 2000, end: 1500, bids: 49, reserve: "Reserve met", pos: "45% 60%" },
    { id: "challenger", img: "/assets/challenger.jpg", alt: "Lime green 1970 Dodge Challenger R/T with a black hood stripe", title: "1970 Dodge Challenger R/T", spec: "426 Hemi, black stripe, Sublime Green", where: "Melbourne, 61,000 km", bid: 168000, step: 2000, end: 21600, bids: 29, reserve: "No reserve", pos: "50% 55%" },
    { id: "boss429", img: "/assets/boss429.jpg", alt: "Red 1969 Ford Mustang Boss 429 with its hood raised", title: "1969 Ford Mustang Boss 429", spec: "429 semi-hemi V8, Candy Apple Red", where: "Charlotte, 54,000 km", bid: 312000, step: 2500, end: 43200, bids: 22, reserve: "Reserve not met", pos: "50% 55%" },
  ],
};
