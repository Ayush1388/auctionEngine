// Editorial content for the event pages. The lot cards are brought to life by the backend.

export const CARS = {
  corvette: { img: "/assets/corvette.jpg", alt: "White 1963 Chevrolet Corvette Sting Ray", title: "1963 Chevrolet Corvette Sting Ray", where: "Sydney, 41,200 km", bid: 118000, step: 1000, bids: 27, reserve: "Reserve met" },
  gullwing: { img: "/assets/300sl.jpg", alt: "Red 1955 Mercedes-Benz 300 SL Gullwing", title: "1955 Mercedes-Benz 300 SL Gullwing", where: "Stuttgart, 62,000 km", bid: 1380000, step: 10000, bids: 31, reserve: "Reserve not met" },
  etype: { img: "/assets/etype.jpg", alt: "Blue 1964 Jaguar E-Type Series 1 coupe", title: "1964 Jaguar E-Type Series 1", where: "Norwich, 58,300 km", bid: 168000, step: 2000, bids: 24, reserve: "Reserve met", pos: "42% 60%" },
  porsche: { img: "/assets/porsche.jpg", alt: "Cream 1972 Porsche 911 Targa", title: "1972 Porsche 911 S Targa", where: "Lippe, 63,800 km", bid: 186000, step: 2000, bids: 41, reserve: "Reserve met" },
  gt500: { img: "/assets/gt500.jpg", alt: "Black 1967 Shelby GT500 KR", title: "1967 Shelby GT500 KR", where: "Nuremberg, 68,000 km", bid: 214000, step: 2000, bids: 49, reserve: "Reserve met", pos: "45% 60%" },
  boss: { img: "/assets/boss429.jpg", alt: "Red 1969 Ford Mustang Boss 429", title: "1969 Ford Mustang Boss 429", where: "Charlotte, 54,000 km", bid: 312000, step: 2500, bids: 22, reserve: "Reserve not met" },
  camaro: { img: "/assets/camaro.jpg", alt: "Black 1967 Chevrolet Camaro SS convertible", title: "1967 Chevrolet Camaro SS 350", where: "Heidelberg, 52,000 km", bid: 78500, step: 1000, bids: 37, reserve: "Reserve met" },
  pontiac: { img: "/assets/ivan.jpg", alt: "Red 1951 Pontiac Chieftain convertible", title: "1951 Pontiac Chieftain", where: "Miami, 88,000 km", bid: 64500, step: 500, bids: 18, reserve: "No reserve" },
  excalibur: { img: "/assets/doina.jpg", alt: "Red Excalibur roadster", title: "1969 Excalibur Roadster", where: "New York, 31,000 km", bid: 71000, step: 1000, bids: 16, reserve: "No reserve", pos: "50% 62%" },
  mustang: { img: "/assets/theodor.jpg", alt: "Red 1965 Ford Mustang coupe", title: "1965 Ford Mustang Coupe", where: "London, 72,400 km", bid: 38000, step: 500, bids: 33, reserve: "No reserve", pos: "50% 64%" },
  impala: { img: "/assets/hero.jpg", alt: "Black 1963 Chevrolet Impala SS", title: "1963 Chevrolet Impala SS", where: "San Diego, 84,500 km", bid: 42500, step: 500, bids: 18, reserve: "No reserve", pos: "50% 70%" },
};

export const EVENTS = {
  retromobile: {
    name: "Rétromobile Paris", short: "Rétromobile", img: "/assets/events/retromobile.jpg", pos: "50% 55%",
    kicker: "Live event", dates: "Feb 3 to 7, 2027  |  Paris Expo, France",
    intro: "Five days of pre-war racers and postwar coachbuilt rarities in a single hall, with the auction held on the Friday night.",
    facts: [["Dates", "Feb 3 to 7, 2027", "5 days"], ["Venue", "Paris Expo", "Porte de Versailles"], ["Lots", "62", "Pre-war to 1975"], ["Estimate", "$18M to $24M", "Combined"]],
    expTitle: "More Than an Auction", expLead: "Walk the halls, meet the specialists and hear the cars run before the sale. Every activity is open to registered bidders.",
    exp: [["Hall Walk", "Preview every lot with the specialist who catalogued it.", "Feb 3 to 6", "Hall 1, Paris Expo", "/assets/exp/35828170.jpg"], ["Concours Display", "Restored cars shown under lights, judged by marque clubs.", "Feb 4 to 7", "Hall 2", "/assets/exp/248687.jpg"], ["Paddock Start-Up", "A scheduled morning when selected lots are fired up in the yard.", "Feb 5", "Outdoor yard", "/assets/exp/190537.jpg"], ["Collectors' Dinner", "A seated dinner for consignors and registered bidders.", "Feb 6", "Pavillon Gabriel", "/assets/exp/5116976.jpg"]], layout: "wave",
    ft: ["gullwing", "Featured lot", "Lot 014, Rétromobile Paris", "A gullwing coupe with its original fitted luggage, shown at the Paris sale for the first time.", 1380000, 31, 18200],
    block: ["etype", "gullwing", "porsche", "corvette"], lead: "A curated selection crossing the block in Paris. Open a lot for the full file, then bid live.",
  },
  "villa-deste": {
    name: "Concorso d'Eleganza Villa d'Este", short: "Villa d'Este", img: "/assets/events/villa-deste.jpg", pos: "50% 60%",
    kicker: "Live event", dates: "Apr 23 to 25, 2027  |  Lake Como, Italy",
    intro: "Concours-grade classics sold on the shore of the lake, in the same gardens where the jury judges the cars.",
    facts: [["Dates", "Apr 23 to 25, 2027", "3 days"], ["Venue", "Villa Erba", "Cernobbio, Lake Como"], ["Lots", "38", "Coachbuilt and concours"], ["Estimate", "$22M to $30M", "Combined"]],
    expTitle: "More Than an Auction", expLead: "A weekend of gardens, judges and long lake-side lunches. Registered bidders get access to every part of it.",
    exp: [["Concours Lawn", "Walk the lawn as the jury judges the finest cars of the weekend.", "Apr 24 to 25", "Villa Erba gardens", "/assets/events/villa-deste.jpg"], ["Coachbuilt Preview", "Study one-off bodies and original paint up close before the sale.", "Apr 23", "Villa Erba", "/assets/exp/28611534.jpg"], ["Lake Drive", "A guided morning drive of selected lots along the western shore.", "Apr 24", "Lake Como", "/assets/exp/29187235.jpg"], ["Gala Dinner", "An evening for bidders and consignors beside the water.", "Apr 24", "Villa d'Este terrace", "/assets/exp/5864590.jpg"]], layout: "wave",
    ft: ["etype", "Featured lot", "Lot 009, Villa d'Este", "A Series 1 coupe in the original colour, with a complete file and one family owner for decades.", 168000, 24, 26400],
    block: ["gullwing", "etype", "excalibur", "pontiac"], lead: "Fine cars for the lake-side sale. Open a lot for the full file, then bid live.",
  },
  monterey: {
    name: "Monterey Car Week", short: "Monterey", img: "/assets/events/pebble-beach.jpg", pos: "50% 55%",
    kicker: "Live event", dates: "Aug 12 to 15, 2027  |  Pebble Beach, California",
    intro: "A global celebration of automotive culture. Rare machines, iconic collectors and the year's biggest sale, timed to the concours.",
    facts: [["Dates", "Aug 12 to 15, 2027", "4 days"], ["Venue", "Pebble Beach", "Monterey, California"], ["Lots", "96", "Iconic cars"], ["Estimate", "$60M to $85M", "Combined"]],
    expTitle: "More Than an Auction", expLead: "Four days of driving, design and exceptional cars. Registered bidders get the paddock, the lawn and the dinner.",
    exp: [["Track Sessions", "Take a passenger lap in selected lots on the circuit.", "Aug 12 to 14", "Laguna Seca", "/assets/exp/9500812.jpg"], ["Concours Display", "Classics, bespoke builds and one-off creations on the fairway.", "Aug 15", "Pebble Beach lawn", "/assets/exp/34262971.jpg"], ["Paddock Walk", "Exclusive access to teams, drivers and historic race cars.", "Aug 13", "Paddock area", "/assets/exp/6154898.jpg"], ["Collectors' Dinner", "An evening with collectors, drivers and industry leaders.", "Aug 14", "Pebble Beach resort", "/assets/exp/5864498.jpg"]], layout: "wave",
    ft: ["gt500", "Featured lot", "Lot 014, Monterey Car Week", "A fastback in period black with its original 428 engine, a fixture of the sale for its condition and file.", 214000, 49, 18480],
    block: ["gt500", "corvette", "porsche", "camaro"], lead: "A curated selection of vehicles crossing the block in Monterey. Open a lot for full details, then bid live.",
  },
  goodwood: {
    name: "Goodwood Revival", short: "Goodwood", img: "/assets/events/goodwood-revival.jpg", pos: "50% 55%",
    kicker: "Live event", dates: "Sep 10 to 12, 2027  |  West Sussex, England",
    intro: "Race and road cars from the golden era, sold the same weekend they take to the circuit in period dress.",
    facts: [["Dates", "Sep 10 to 12, 2027", "3 days"], ["Venue", "Goodwood Motor Circuit", "Chichester, West Sussex"], ["Lots", "48", "1950s to 1960s"], ["Estimate", "$16M to $22M", "Combined"]],
    expTitle: "More Than an Auction", expLead: "The whole estate dresses for the 1960s. Registered bidders can see the cars run and then bid on them.",
    exp: [["Circuit Day", "Watch selected lots run on the track in period race meetings.", "Sep 10 to 12", "Goodwood circuit", "/assets/events/goodwood-revival.jpg"], ["Paddock Preview", "Walk the paddock with the owners and the specialists.", "Sep 10", "Paddock", "/assets/exp/29198149.jpg"], ["Period Village", "Vintage shops, stalls and a full 1960s high street.", "All weekend", "Goodwood village", "/assets/events/pebble-beach.jpg"], ["Dinner on the Lawn", "A supper for bidders and consignors with the cars on show.", "Sep 11", "House lawn", "/assets/exp/8995669.jpg"]], layout: "wave",
    ft: ["etype", "Featured lot", "Lot 021, Goodwood Revival", "A race-prepared coupe with period history, eligible for the weekend's own races.", 168000, 24, 21100],
    block: ["etype", "gt500", "boss", "mustang"], lead: "Cars chosen for the revival sale. Open a lot for the full file, then bid live.",
  },
};
