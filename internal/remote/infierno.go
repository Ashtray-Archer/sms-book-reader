package remote

// Spanish and Italian come from walnut-burgundy/mi-Dante at 40a1a17.
// The notes follow lectura/Infierno/01.md and 01.013-030.md. The two
// editions align by sense: several tercets can correspond to one sentence.
type comparison struct {
	spanish, italian, verses, note string
	working                        string // attested Spanish working version, where the reading project has one
}

var infiernoOpening = []comparison{
	{
		spanish: "A la mitad del viaje de nuestra vida me encontré en una selva obscura, por haberme apartado del camino recto.",
		italian: `Nel mezzo del cammin di nostra vita
mi ritrovai per una selva oscura,
ché la diritta via era smarrita.`,
		verses: "1-3",
		note:   "«Viaje» nombra el curso de la vida. Para la senda perdida, Aranda elige otra palabra, «camino». En «por haberme apartado», Dante se atribuye el extravío.",
	},
	{
		spanish: "¡Ah! Cuán penoso me sería decir lo salvaje, áspera y espesa que era esta selva, cuyo recuerdo renueva mi pavor, pavor tan amargo, que la muerte no lo es tanto. Pero antes de hablar del bien que allí encontré, revelaré las demás cosas que he visto.",
		italian: `Ahi quanto a dir qual era è cosa dura
esta selva selvaggia e aspra e forte
che nel pensier rinova la paura!

Tant’ è amara che poco è più morte;
ma per trattar del ben ch’i’ vi trovai,
dirò de l’altre cose ch’i’ v’ho scorte.`,
		verses: "4-9",
		note:   "«Espesa» vuelve física la dificultad de atravesar la selva. En la prosa de Aranda, lo amargo es el pavor que vuelve al recordarla.",
	},
	{
		spanish: "No sé decir fijamente cómo entré allí; tan adormecido estaba cuando abandoné el verdadero camino.",
		italian: `Io non so ben ridir com’ i’ v’intrai,
tant’ era pien di sonno a quel punto
che la verace via abbandonai.`,
		verses: "10-12",
		note:   "«La verace via abbandonai» cierra el terceto. Aranda recupera «camino», la palabra de la primera frase.",
	},
	{
		spanish: "Pero al llegar al pie de una cuesta, donde terminaba el valle que me había llenado de miedo el corazón, miré hacia arriba, y vi su cima revestida ya de los rayos del planeta que nos guía con seguridad por todos los senderos.",
		italian: `Ma poi ch’i’ fui al piè d’un colle giunto,
là dove terminava quella valle
che m’avea di paura il cor compunto,

guardai in alto e vidi le sue spalle
vestite già de’ raggi del pianeta
che mena dritto altrui per ogne calle.`,
		verses:  "13-18",
		working: "Pero cuando llegué al pie de una colina, allí donde terminaba el valle que me había traspasado de miedo el corazón, miré hacia arriba y vi sus hombros vestidos ya de los rayos del planeta que guía derecho a todos por cualquier senda.",
		note:    "Dante ve los «hombros» del monte vestidos de luz. Aranda escribe «cima» y sitúa la mirada en lo más alto.",
	},
	{
		spanish: "Entonces se calmó algún tanto el miedo que había permanecido en el lago de mi corazón durante la noche que pasé con tanta angustia;",
		italian: `Allor fu la paura un poco queta,
che nel lago del cor m’era durata
la notte ch’i’ passai con tanta pieta.`,
		verses:  "19-21",
		working: "Entonces se aquietó un poco el miedo que había durado en el lago de mi corazón durante la noche que pasé con tanta angustia.",
		note:    "Aranda conserva «lago del cor» como «lago de mi corazón». Para «pieta» elige «angustia».",
	},
	{
		spanish: "y del mismo modo que aquel que, saliendo anhelante fuera del piélago, al llegar a la playa, se vuelve hacia las ondas peligrosas y las contempla, así mi espíritu, fugitivo aún, se volvió hacia atrás para mirar el lugar de que no salió nunca nadie vivo.",
		italian: `E come quei che con lena affannata,
uscito fuor del pelago a la riva,
si volge a l’acqua perigliosa e guata,

così l’animo mio, ch’ancor fuggiva,
si volse a retro a rimirar lo passo
che non lasciò già mai persona viva.`,
		verses:  "22-27",
		working: "Y como quien, con aliento jadeante, salido del piélago a la orilla, se vuelve y mira las aguas peligrosas, así mi ánimo, que aún huía, se volvió atrás a mirar el paso que jamás dejó salir viva a persona alguna.",
		note:    "El náufrago vuelve la vista al agua desde la orilla. También el espíritu de Dante, todavía en fuga, se vuelve a mirar el paso que acaba de dejar.",
	},
	{
		spanish: "Después de haber dado algún reposo a mi fatigado cuerpo, continué subiendo por la solitaria playa, procurando afirmar siempre aquel de mis pies que estuviera más bajo.",
		italian: `Poi ch’èi posato un poco il corpo lasso,
ripresi via per la piaggia diserta,
sì che ’l piè fermo sempre era ’l più basso.`,
		verses:  "28-30",
		working: "Después de reposar un poco el cuerpo cansado, retomé el camino por la ladera desierta, con el pie firme siempre más bajo.",
		note:    "«Piaggia diserta» es aquí la ladera desierta. «Playa» prolonga la imagen marina anterior, aunque Dante ya está subiendo.",
	},
}
