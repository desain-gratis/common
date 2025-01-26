
CREATE TABLE public.gsi_authorized_user (
    gid SERIAL,
    namespace character varying NOT NULL,
    id character varying NOT NULL,
    data jsonb NOT NULL,
    meta jsonb NOT NULL
);
ALTER TABLE ONLY public.gsi_authorized_user
    ADD CONSTRAINT gsi_authorized_user_pkey PRIMARY KEY (namespace, id);

CREATE TABLE public.gsi_authorized_user_thumbnail (
    gid SERIAL,
    namespace character varying NOT NULL,
    ref_id_1 character varying, -- refer to gsi_authorized_user id
    id character varying NOT NULL,
    data jsonb NOT NULL,
    meta jsonb NOT NULL
);
ALTER TABLE ONLY public.gsi_authorized_user_thumbnail
    ADD CONSTRAINT gsi_authorized_user_thumbnail_pkey PRIMARY KEY (namespace, id, ref_id_1);
